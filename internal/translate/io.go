package translate

import "os"

// readFile is a seam so tests can stay off the filesystem where useful.
func readFile(p string) (string, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
