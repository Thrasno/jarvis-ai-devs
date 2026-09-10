package applyprogress

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSealBatchCanonicalAndBounded(t *testing.T) {
	for _, tt := range []struct {
		name    string
		batch   Batch
		wantErr error
	}{
		{
			name:  "seals canonical batch deterministically",
			batch: validBatch("<evidence>"),
		},
		{
			name: "rejects invalid batch ID",
			batch: Batch{
				Schema:  EvidenceSchema,
				Project: "jarvis-dev",
				Change:  "issue-653",
				BatchID: "not a batch id",
				Entries: []EvidenceEntry{},
			},
			wantErr: ErrInvalidID,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sealed, bytes, err := SealBatch(tt.batch)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SealBatch() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(bytes), "\\u003c") || strings.Contains(string(bytes), ": ") || strings.Contains(string(bytes), ", ") {
				t.Fatalf("SealBatch() = %q, want compact JSON without HTML escaping", bytes)
			}
			if len(sealed.SHA256) != 64 {
				t.Fatalf("SHA256 = %q, want SHA-256 hex", sealed.SHA256)
			}
			_, again, err := SealBatch(tt.batch)
			if err != nil || string(bytes) != string(again) {
				t.Fatalf("SealBatch() is not deterministic: %q != %q, error %v", bytes, again, err)
			}
		})
	}

	for _, target := range []int{MaxDocumentRunes, MaxDocumentRunes + 1} {
		t.Run("enforces final rune boundary", func(t *testing.T) {
			batch := validBatch("")
			_, bytes, err := SealBatch(batch)
			if err != nil {
				t.Fatal(err)
			}
			batch.Entries[0].Summary = strings.Repeat("é", target-utf8.RuneCountInString(string(bytes)))
			_, bytes, err = SealBatch(batch)
			if target == MaxDocumentRunes {
				if err != nil || utf8.RuneCountInString(string(bytes)) != target {
					t.Fatalf("SealBatch() runes/error = %d/%v, want %d/nil", utf8.RuneCountInString(string(bytes)), err, target)
				}
				return
			}
			var capacity *CapacityError
			if !errors.As(err, &capacity) || capacity.Runes != target {
				t.Fatalf("SealBatch() error = %#v, want capacity at %d", err, target)
			}
		})
	}
}

func TestCanonicalDecodingIntegrityAndSnapshotBounds(t *testing.T) {
	batch, raw, err := SealBatch(validBatch("evidence"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		data []byte
		want error
	}{
		{"non-canonical whitespace", append(append([]byte{}, raw...), ' '), ErrNonCanonical},
		{"invalid UTF-8", append(append([]byte{}, raw[:10]...), 0xff), ErrNonCanonical},
		{"invalid ID", []byte(strings.Replace(string(raw), "1A.1", "!bad", 1)), ErrInvalidID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeCanonicalBatch(tt.data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("DecodeCanonicalBatch() error = %v, want %v", err, tt.want)
			}
		})
	}
	batch.Entries[0].Summary = "tampered"
	if err := VerifyBatch(batch); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("VerifyBatch() error = %v, want hash mismatch", err)
	}

	for _, target := range []int{MaxDocumentRunes, MaxDocumentRunes + 1} {
		t.Run("enforces snapshot rune boundary", func(t *testing.T) {
			snapshot := snapshotAtSize(t, target)
			sealed, bytes, err := SealSnapshot(snapshot)
			if target == MaxDocumentRunes {
				if err != nil || utf8.RuneCountInString(string(bytes)) != target {
					t.Fatalf("SealSnapshot() runes/error = %d/%v, want %d/nil", utf8.RuneCountInString(string(bytes)), err, target)
				}
				if err := VerifySnapshot(sealed); err != nil {
					t.Fatalf("VerifySnapshot() error = %v", err)
				}
				return
			}
			var capacity *CapacityError
			if !errors.As(err, &capacity) || capacity.Runes != target {
				t.Fatalf("SealSnapshot() error = %#v, want capacity at %d", err, target)
			}
		})
	}
}

func TestDecodeCanonicalSnapshot(t *testing.T) {
	snapshot, data, err := SealSnapshot(validSnapshot(""))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalSnapshot(data)
	if err != nil || !reflect.DeepEqual(decoded, snapshot) {
		t.Fatalf("DecodeCanonicalSnapshot() = %#v, %v; want %#v, nil", decoded, err, snapshot)
	}

	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"whitespace", append(append([]byte{}, data...), ' ')},
		{"BOM", append([]byte{0xef, 0xbb, 0xbf}, data...)},
		{"unknown field", append(append([]byte{}, data[:len(data)-1]...), []byte(`,"unknown":true}`)...)},
		{"duplicate field", append(append([]byte{}, data[:len(data)-1]...), []byte(`,"schema":"jarvis.sdd-apply-progress/v2"}`)...)},
		{"trailing value", append(append([]byte{}, data...), data...)},
	} {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			if _, err := DecodeCanonicalSnapshot(tt.data); !errors.Is(err, ErrNonCanonical) {
				t.Fatalf("DecodeCanonicalSnapshot() error = %v, want %v", err, ErrNonCanonical)
			}
		})
	}

	snapshot.Status = StatusComplete
	if err := VerifySnapshot(snapshot); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("VerifySnapshot() error = %v, want %v", err, ErrHashMismatch)
	}
}

func TestRejectsMalformedProtocolValues(t *testing.T) {
	for _, tt := range []struct {
		name     string
		validate func() error
		want     error
	}{
		{"batch schema", func() error { b := validBatch(""); b.Schema = "v1"; _, _, err := SealBatch(b); return err }, ErrInvalidSchema},
		{"evidence kind", func() error { b := validBatch(""); b.Entries[0].Kind = "bad"; _, _, err := SealBatch(b); return err }, ErrInvalidValue},
		{"evidence outcome", func() error { b := validBatch(""); b.Entries[0].Outcome = "bad"; _, _, err := SealBatch(b); return err }, ErrInvalidValue},
		{"snapshot schema", func() error { s := validSnapshot(""); s.Schema = "v1"; _, _, err := SealSnapshot(s); return err }, ErrInvalidSchema},
		{"snapshot status", func() error { s := validSnapshot(""); s.Status = "bad"; _, _, err := SealSnapshot(s); return err }, ErrInvalidValue},
		{"previous digest", func() error { _, _, err := SealSnapshot(validSnapshot("not-a-digest")); return err }, ErrHashMismatch},
		{"task manifest digest", func() error {
			s := validSnapshot("")
			s.TaskManifestSHA256 = strings.Repeat("A", 64)
			_, _, err := SealSnapshot(s)
			return err
		}, ErrHashMismatch},
		{"batch reference digest", func() error {
			s := validSnapshot("")
			s.Batches[0].SHA256 = strings.Repeat("z", 64)
			_, _, err := SealSnapshot(s)
			return err
		}, ErrHashMismatch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.validate(); !errors.Is(err, tt.want) {
				t.Fatalf("validation error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRejectsInvalidUTF8TypedInput(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value func() reflect.Value
		seal  func(reflect.Value) error
	}{
		{"batch", func() reflect.Value { b := validBatch(""); return reflect.ValueOf(&b).Elem() }, func(v reflect.Value) error { _, _, err := SealBatch(v.Interface().(Batch)); return err }},
		{"snapshot", func() reflect.Value { s := validSnapshot(""); return reflect.ValueOf(&s).Elem() }, func(v reflect.Value) error { _, _, err := SealSnapshot(v.Interface().(Snapshot)); return err }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.value()
			for index, field := range stringFields(root) {
				original := field.String()
				field.SetString(string([]byte{0xff}))
				if err := tt.seal(root); !errors.Is(err, ErrInvalidValue) {
					t.Fatalf("string field %d validation error = %v, want %v", index, err, ErrInvalidValue)
				}
				field.SetString(original)
			}
		})
	}
}

func stringFields(value reflect.Value) []reflect.Value {
	var fields []reflect.Value
	var visit func(reflect.Value)
	visit = func(value reflect.Value) {
		switch value.Kind() {
		case reflect.String:
			fields = append(fields, value)
		case reflect.Struct:
			for index := range value.NumField() {
				visit(value.Field(index))
			}
		case reflect.Slice:
			for index := range value.Len() {
				visit(value.Index(index))
			}
		}
	}
	visit(value)
	return fields
}

func snapshotAtSize(t *testing.T, target int) Snapshot {
	t.Helper()
	snapshot := validSnapshot("")
	for {
		snapshot.Coverage = append(snapshot.Coverage, Coverage{
			TaskID: "a", BatchID: "apb-0123456789abcdef0123456789abcdef", EntryID: "e",
		})
		_, data, err := SealSnapshot(snapshot)
		runes := utf8.RuneCount(data)
		if err != nil {
			var capacity *CapacityError
			if !errors.As(err, &capacity) {
				t.Fatal(err)
			}
			runes = capacity.Runes
		}
		gap := target - runes
		if gap >= 0 && gap <= 126 {
			growth := min(gap, 63)
			snapshot.Coverage[len(snapshot.Coverage)-1].TaskID = strings.Repeat("a", growth+1)
			snapshot.Coverage[len(snapshot.Coverage)-1].EntryID = strings.Repeat("e", gap-growth+1)
			return snapshot
		}
	}
}

func validBatch(summary string) Batch {
	return Batch{
		Schema:  EvidenceSchema,
		Project: "jarvis-dev",
		Change:  "issue-653",
		BatchID: "apb-0123456789abcdef0123456789abcdef",
		Entries: []EvidenceEntry{{
			EntryID:          "1A.1",
			TaskIDs:          []string{"1A.1"},
			CompletesTaskIDs: []string{"1A.1"},
			Kind:             EvidenceRed,
			Summary:          summary,
			Command:          "go test ./applyprogress",
			ExitCode:         1,
			Outcome:          OutcomeFail,
			Files:            []string{"canonical_test.go"},
		}},
	}
}

func validSnapshot(previousDigest string) Snapshot {
	return Snapshot{
		Schema:             SnapshotSchema,
		Project:            "jarvis-dev",
		Change:             "issue-653",
		PreviousDigest:     previousDigest,
		TaskManifestSHA256: strings.Repeat("a", 64),
		Status:             StatusPartial,
		Coverage:           []Coverage{},
		Batches: []BatchRef{{
			BatchID: "apb-0123456789abcdef0123456789abcdef",
			SHA256:  strings.Repeat("b", 64),
		}},
	}
}
