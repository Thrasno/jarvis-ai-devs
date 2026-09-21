//go:build windows

package sddbinding

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type changeRoot struct {
	root      *os.Root
	mutexName string
}

type stateRecord struct {
	data   []byte
	mode   os.FileMode
	exists bool
	info   os.FileInfo
}

func openChangeRoot(changeDir string) (*changeRoot, error) {
	if strings.TrimSpace(changeDir) == "" {
		return nil, fmt.Errorf("%w: empty change directory", ErrUnsafePath)
	}
	path, err := filepath.Abs(filepath.Clean(changeDir))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("%w: change directory", ErrUnsafePath)
	}
	mutexName, err := rootMutexName(root)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &changeRoot{root: root, mutexName: mutexName}, nil
}

func rootMutexName(root *os.Root) (string, error) {
	file, err := root.Open(".")
	if err != nil {
		return "", fmt.Errorf("%w: open change directory identity", ErrUnsafePath)
	}
	defer file.Close()
	connection, err := file.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("%w: change directory identity", ErrUnsafePath)
	}
	var info windows.ByHandleFileInformation
	var handleErr error
	if err := connection.Control(func(handle uintptr) {
		handleErr = windows.GetFileInformationByHandle(windows.Handle(handle), &info)
	}); err != nil || handleErr != nil {
		return "", fmt.Errorf("%w: change directory identity", ErrUnsafePath)
	}
	identity := fmt.Sprintf("%08x:%08x:%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow)
	digest := sha256.Sum256([]byte(identity))
	return "Global\\jarvis-sddbinding-" + hex.EncodeToString(digest[:]), nil
}

func (r *changeRoot) Close() error { return r.root.Close() }

func (r *changeRoot) samePhysicalDirectory(other *changeRoot) (bool, error) {
	return r.mutexName == other.mutexName, nil
}

func (r *changeRoot) lock() (func(), error) {
	name, err := windows.UTF16PtrFromString(r.mutexName)
	if err != nil {
		return nil, fmt.Errorf("lock OpenSpec binding directory: %w", err)
	}
	handle, createErr := windows.CreateMutex(nil, false, name)
	if createErr != nil && !errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, fmt.Errorf("lock OpenSpec binding directory: %w", createErr)
	}
	if handle == 0 {
		return nil, errors.New("lock OpenSpec binding directory: invalid mutex handle")
	}

	runtime.LockOSThread()
	result, waitErr := windows.WaitForSingleObject(handle, windows.INFINITE)
	if waitErr != nil || (result != windows.WAIT_OBJECT_0 && result != windows.WAIT_ABANDONED) {
		runtime.UnlockOSThread()
		_ = windows.CloseHandle(handle)
		if waitErr != nil {
			return nil, fmt.Errorf("lock OpenSpec binding directory: %w", waitErr)
		}
		return nil, errors.New("lock OpenSpec binding directory: unexpected wait result")
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			_ = windows.ReleaseMutex(handle)
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
		})
	}, nil
}

func (r *changeRoot) readState() (stateRecord, error) {
	info, err := r.root.Lstat(stateFileName)
	if errors.Is(err, os.ErrNotExist) {
		return stateRecord{}, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return stateRecord{}, fmt.Errorf("%w: state.yaml", ErrUnsafePath)
	}
	file, err := r.root.Open(stateFileName)
	if err != nil {
		return stateRecord{}, fmt.Errorf("%w: state.yaml", ErrUnsafePath)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return stateRecord{}, fmt.Errorf("%w: state.yaml", ErrUnsafePath)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return stateRecord{}, fmt.Errorf("read OpenSpec binding state: %w", err)
	}
	return stateRecord{data: data, mode: info.Mode().Perm(), exists: true, info: info}, nil
}

func (r *changeRoot) replaceState(data []byte, prior stateRecord) error {
	current, err := r.root.Lstat(stateFileName)
	if prior.exists {
		if err != nil || !current.Mode().IsRegular() || !os.SameFile(prior.info, current) {
			return fmt.Errorf("%w: state.yaml changed before commit", ErrUnsafePath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: state.yaml appeared before commit", ErrUnsafePath)
	}
	mode := prior.mode
	if !prior.exists {
		mode = 0o600
	}
	name, file, err := r.createTemp()
	if err != nil {
		return err
	}
	defer func() { _ = r.root.Remove(name) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write OpenSpec binding stage: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync OpenSpec binding stage: %w", err)
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("set OpenSpec binding mode: %w", err)
	}
	if err := r.renameStaged(file); err != nil {
		_ = file.Close()
		return err
	}
	_ = file.Close()
	// NtSetInformationFile is the commit point. os.Root exposes no directory fsync,
	// so callers receive success after commit rather than a misleading error.
	return nil
}

// fileRenameInformationEx is the NT FILE_INFORMATION_CLASS value for
// FILE_RENAME_INFORMATION_EX. golang.org/x/sys/windows does not export it.
const fileRenameInformationEx uint32 = 65

type fileRenameInfoEx struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

func stagedRenameFlags() uint32 {
	return windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS | windows.FILE_RENAME_IGNORE_READONLY_ATTRIBUTE
}

func fileRenameInfoExBuffer(directoryHandle windows.Handle, target []uint16) []byte {
	nameOffset := int(unsafe.Offsetof(fileRenameInfoEx{}.FileName))
	bufferSize := nameOffset + len(target)*2
	if minimum := int(unsafe.Sizeof(fileRenameInfoEx{})); bufferSize < minimum {
		bufferSize = minimum
	}
	buffer := make([]byte, bufferSize)
	info := (*fileRenameInfoEx)(unsafe.Pointer(&buffer[0]))
	info.Flags = stagedRenameFlags()
	info.RootDirectory = directoryHandle
	info.FileNameLength = uint32((len(target) - 1) * 2)
	copy(unsafe.Slice(&info.FileName[0], len(target)), target)
	return buffer
}

func (r *changeRoot) renameStaged(file *os.File) error {
	stagedHandle, err := fileHandle(file)
	if err != nil {
		return err
	}
	directory, err := r.root.Open(".")
	if err != nil {
		return fmt.Errorf("open OpenSpec binding directory for commit: %w", err)
	}
	defer directory.Close()
	directoryHandle, err := fileHandle(directory)
	if err != nil {
		return err
	}
	target, err := windows.UTF16FromString(stateFileName)
	if err != nil {
		return fmt.Errorf("encode OpenSpec binding target: %w", err)
	}
	buffer := fileRenameInfoExBuffer(directoryHandle, target)
	var status windows.IO_STATUS_BLOCK
	renameErr := windows.NtSetInformationFile(stagedHandle, &status, &buffer[0], uint32(len(buffer)), fileRenameInformationEx)
	runtime.KeepAlive(file)
	runtime.KeepAlive(directory)
	if renameErr != nil {
		return fmt.Errorf("replace OpenSpec binding state: %w", renameErr)
	}
	return nil
}

func fileHandle(file *os.File) (windows.Handle, error) {
	connection, err := file.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("access OpenSpec binding handle: %w", err)
	}
	var handle windows.Handle
	if err := connection.Control(func(raw uintptr) { handle = windows.Handle(raw) }); err != nil {
		return 0, fmt.Errorf("access OpenSpec binding handle: %w", err)
	}
	return handle, nil
}

func (r *changeRoot) createTemp() (string, *os.File, error) {
	directory, err := r.root.Open(".")
	if err != nil {
		return "", nil, fmt.Errorf("open OpenSpec binding directory for staging: %w", err)
	}
	defer directory.Close()
	directoryHandle, err := fileHandle(directory)
	if err != nil {
		return "", nil, err
	}
	for range 10 {
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", nil, fmt.Errorf("create OpenSpec binding stage: %w", err)
		}
		name := ".state.yaml.sdd-binding-" + hex.EncodeToString(suffix[:])
		objectName, err := windows.NewNTUnicodeString(name)
		if err != nil {
			return "", nil, fmt.Errorf("create OpenSpec binding stage: %w", err)
		}
		attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: directoryHandle, ObjectName: objectName}
		var status windows.IO_STATUS_BLOCK
		var allocation int64
		var handle windows.Handle
		err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, &attributes, &status, &allocation, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_CREATE, 0, 0, 0)
		if errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create OpenSpec binding stage: %w", err)
		}
		return name, os.NewFile(uintptr(handle), name), nil
	}
	return "", nil, fmt.Errorf("create OpenSpec binding stage: exhausted unique names")
}
