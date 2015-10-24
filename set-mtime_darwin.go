package git

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func touchDatetime(t time.Time) string {
	return fmt.Sprintf("%04d%02d%02d%02d%02d.%02d",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}

// Chtimes changes the modification and access time of a given file.
func Chtimes(path string, atime, mtime time.Time) error {
	cmd := exec.Command("touch", "-a", "-h", "-t", touchDatetime(atime), path)
	if e := cmd.Run(); e != nil {
		return &os.PathError{Op: "futimesat", Path: path, Err: e}
	}

	cmd = exec.Command("touch", "-m", "-h", "-t", touchDatetime(mtime), path)
	if e := cmd.Run(); e != nil {
		return &os.PathError{Op: "futimesat", Path: path, Err: e}
	}

	return nil
}
