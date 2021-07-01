package path

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Convert(pathSeparator rune, path string, osFn func (path string) string) string {
	if os.PathSeparator != pathSeparator {
		if os.IsPathSeparator('/') || os.IsPathSeparator('\\') {
			path = ToOsSeparator(pathSeparator, path)
			path = osFn(path)
			return FromOsSeparator(pathSeparator, path)

		} else {
			panic(fmt.Sprintf("invalid path separator: '%s'", string(pathSeparator)))
		}
	}

	return osFn(path)
}

func ToOsSeparator(pathSeparator rune, path string) string {
	return strings.ReplaceAll(path, string(pathSeparator), string(os.PathSeparator))
}

func FromOsSeparator(pathSeparator rune, path string) string {
	return strings.ReplaceAll(path, string(os.PathSeparator), string(pathSeparator))
}

func Join(pathSeparator rune, pathParts ...string) string {
	if os.PathSeparator != pathSeparator {
		if os.IsPathSeparator('/') || os.IsPathSeparator('\\') {
			for i, pathPart := range pathParts {
				pathParts[i] = ToOsSeparator(pathSeparator, pathPart)
			}
			return FromOsSeparator(pathSeparator, filepath.Join(pathParts...))
		}
	}

	pathSeparatorString := string(pathSeparator)
	joinedPath := strings.Join(pathParts, pathSeparatorString)
	return strings.ReplaceAll(joinedPath, pathSeparatorString + pathSeparatorString, pathSeparatorString)
}
