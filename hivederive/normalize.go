package hivederive

import (
	"os"
	"runtime"
	"strings"
	"sync"
)

// runtimeGOOS and wslMarkerFn gate WSL/Windows path rewriting. They are package
// vars so tests can simulate native-Windows, native-Linux, and WSL runtimes
// without a real mount. wslMarkerFn caches its /proc/version probe.
var (
	runtimeGOOS = runtime.GOOS
	wslMarkerFn = detectWSLMarker
)

// NormalizePath translates Windows and WSL path forms into a form resolvable on
// the current runtime, but ONLY when running as a WSL-hosted Linux daemon
// (GOOS=linux with a WSL marker present). On native Windows and native Linux it
// returns dir unchanged, so a native-Windows daemon never mistranslates a
// legitimate C:\ path and a native-Linux daemon never rewrites a backslash that
// is a valid filename character.
//
// Translations, when the gate is open:
//   - C:\Users\dev\project        -> /mnt/c/Users/dev/project
//   - \\wsl$\Distro\home\dev\p    -> /home/dev/p
//   - \\wsl.localhost\Distro\...  -> /...
//   - project\subdir              -> project/subdir
func NormalizePath(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", ErrEmptyDir
	}
	if !shouldNormalize() {
		return dir, nil
	}
	return translateToWSL(dir), nil
}

// shouldNormalize reports whether WSL/Windows rewriting applies to the current
// runtime.
func shouldNormalize() bool {
	return runtimeGOOS == "linux" && wslMarkerFn()
}

// translateToWSL rewrites a Windows or UNC WSL path into its POSIX equivalent.
// It assumes the caller has already decided translation applies.
func translateToWSL(dir string) string {
	// UNC WSL forms: drop the \\server\distro prefix, keep the rest.
	for _, prefix := range []string{`\\wsl$\`, `\\wsl.localhost\`} {
		if strings.HasPrefix(dir, prefix) {
			rest := strings.ReplaceAll(dir[len(prefix):], `\`, "/")
			// rest is "Distro/home/dev/project"; drop the distro segment.
			if idx := strings.Index(rest, "/"); idx >= 0 {
				return "/" + rest[idx+1:]
			}
			return "/"
		}
	}
	// Drive-letter form: C:\... or C:/... -> /mnt/c/...
	if len(dir) >= 2 && isDriveLetter(dir[0]) && dir[1] == ':' {
		drive := strings.ToLower(string(dir[0]))
		rest := strings.TrimPrefix(strings.ReplaceAll(dir[2:], `\`, "/"), "/")
		if rest == "" {
			return "/mnt/" + drive
		}
		return "/mnt/" + drive + "/" + rest
	}
	// Plain backslash-separated path.
	return strings.ReplaceAll(dir, `\`, "/")
}

// isDriveLetter reports whether b is an ASCII letter usable as a drive letter.
func isDriveLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// wslOnce guards the cached WSL-marker probe.
var (
	wslOnce   sync.Once
	wslCached bool
)

// detectWSLMarker reports whether /proc/version identifies a WSL kernel. The
// result is read once and cached; a missing or unreadable /proc/version means
// "not WSL".
func detectWSLMarker() bool {
	wslOnce.Do(func() {
		data, err := os.ReadFile("/proc/version")
		if err != nil {
			wslCached = false
			return
		}
		lower := strings.ToLower(string(data))
		wslCached = strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
	})
	return wslCached
}
