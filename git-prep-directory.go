package git

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// BuildDirectory holds the git rev and path to a cloned git repository. It also
// holds a cleanup function to safely remove this directory.
type BuildDirectory struct {
	Name    string
	Dir     string
	Cleanup func()
}

// PrepBuildDirectory clones a given repository and checks out the given
// revision, setting the timestamp of all files to their commit time and putting
// all submodules into a submodule cache.
func PrepBuildDirectory(gitDir, remote, ref string, timeout time.Duration) (*BuildDirectory, error) {
	start := time.Now()
	defer func() {
		log.Printf("Took %v to prep %v", time.Since(start), remote)
	}()

	if strings.HasPrefix(remote, "github.com/") {
		remote = "https://" + remote
	}

	gitDir, err := filepath.Abs(gitDir)
	if err != nil {
		return nil, fmt.Errorf("unable to determine abspath: %v", err)
	}

	err = LocalMirror(remote, gitDir, ref, timeout, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("unable to LocalMirror: %v", err)
	}

	rev, err := RevParse(gitDir, ref)
	if err != nil {
		return nil, fmt.Errorf("unable to parse rev: %v", err)
	}

	tagName, err := Describe(gitDir, rev)
	if err != nil {
		return nil, fmt.Errorf("unable to describe %v: %v", rev, err)
	}

	shortRev := rev[:10]
	checkoutPath := path.Join(gitDir, filepath.Join("c/", shortRev))

	err = RecursiveCheckout(gitDir, checkoutPath, rev, timeout)
	if err != nil {
		return nil, err
	}

	cleanup := func() {
		err := SafeCleanup(checkoutPath)
		if err != nil {
			log.Println("Error cleaning up path:", checkoutPath)
		}
	}

	return &BuildDirectory{tagName, checkoutPath, cleanup}, nil
}

// SafeCleanup recursively removes all files from a given path, which has to be
// a subfolder of the current working directory.
func SafeCleanup(path string) error {
	if path == "/" || path == "" || path == "." || strings.Contains(path, "..") {
		return fmt.Errorf("invalid path specified for deletion %q", path)
	}
	return os.RemoveAll(path)
}
