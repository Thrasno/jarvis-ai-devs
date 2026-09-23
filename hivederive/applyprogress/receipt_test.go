package applyprogress

import "testing"

// Historical daemon receipt wire JSON encodes byte slices as base64 and fixes field order.
func TestAdvanceReceiptDigestHistoricalDaemonGolden(t *testing.T) {
	const expected = "4d8ab7a99f131ed78c20a3edca07ada4b6b6eef83a48590614a4c5ad60ba1a12"
	got := AdvanceReceiptDigest("p", "c", "r", 2, 3, "head", "", []byte(`{"a":1}`), [][]byte{[]byte(`{"b":2}`)})
	if got != expected {
		t.Fatalf("historical daemon receipt digest = %s, want %s", got, expected)
	}
}

func TestAdvanceReceiptDigestBindsRequestAndCanonicalBytes(t *testing.T) {
	base := AdvanceReceiptDigest("p", "c", "one", 1, 2, "head", "", []byte(`{"a":1}`), [][]byte{[]byte(`{"b":2}`)})
	if len(base) != 64 {
		t.Fatalf("digest=%q", base)
	}
	for _, changed := range []string{
		AdvanceReceiptDigest("p", "c", "two", 1, 2, "head", "", []byte(`{"a":1}`), [][]byte{[]byte(`{"b":2}`)}),
		AdvanceReceiptDigest("p", "c", "one", 1, 2, "head", "", []byte(`{"a":2}`), [][]byte{[]byte(`{"b":2}`)}),
		AdvanceReceiptDigest("p", "c", "one", 1, 2, "head", "", []byte(`{"a":1}`), [][]byte{[]byte(`{"b":3}`)}),
	} {
		if base == changed {
			t.Fatal("receipt identity omitted request input")
		}
	}
}
