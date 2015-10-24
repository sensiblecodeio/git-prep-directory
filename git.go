package git

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/context"
)

// Command invokes a `command` in `workdir` with `args`, connecting Stdout and
// Stderr to Stderr.
func Command(workdir, command string, args ...string) *exec.Cmd {
	// log.Printf("wd = %s cmd = %s, args = %q", workdir, command, append([]string{}, args...))
	cmd := exec.Command(command, args...)
	cmd.Dir = workdir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd
}

// LocalMirror creates or updates a mirror of `url` at `gitDir` using `git clone
// --mirror`.
func LocalMirror(url, gitDir, ref string, messages io.Writer) error {
	// When mirroring, allow up to two minutes before giving up.
	const MirrorTimeout = 2 * time.Minute
	ctx, done := context.WithTimeout(context.Background(), MirrorTimeout)
	defer done()

	if _, err := os.Stat(gitDir); err == nil {
		// Repo already exists, don't need to clone it.

		if AlreadyHaveRef(gitDir, ref) {
			// Sha already exists, don't need to fetch.
			// log.Printf("Already have ref: %v %v", gitDir, ref)
			return nil
		}

		return Fetch(ctx, gitDir, url, messages)
	}

	err := os.MkdirAll(filepath.Dir(gitDir), 0777)
	if err != nil {
		return err
	}

	return Clone(ctx, url, gitDir, messages)
}

// Clone clones a git repository as mirror.
func Clone(ctx context.Context, url, gitDir string, messages io.Writer) error {
	cmd := Command(".", "git", "clone", "-q", "--mirror", url, gitDir)
	cmd.Stdout = messages
	cmd.Stderr = messages
	return ContextRun(ctx, cmd)
}

// ContextRun runs cmd within a net Context.
// If the context is cancelled or times out, the process is killed.
func ContextRun(ctx context.Context, cmd *exec.Cmd) error {
	errc := make(chan error)

	err := cmd.Start()
	if err != nil {
		return err
	}

	go func() { errc <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case err := <-errc:
		return err // err may be nil
	}
}

// Fetch fetches all branches from a given remote.
func Fetch(ctx context.Context, gitDir, url string, messages io.Writer) error {
	cmd := Command(gitDir, "git", "fetch", "-f", url, "*:*")
	cmd.Stdout = messages
	cmd.Stderr = messages

	err := ContextRun(ctx, cmd)
	if err != nil {
		// git fetch where there is no update is exit status 1.
		if err.Error() != "exit status 1" {
			return err
		}
	}

	return nil
}

// ShaLike specifies a valid git hash.
var ShaLike = regexp.MustCompile("[0-9a-zA-Z]{40}")

// AlreadyHaveRef returns true if ref is sha-like and is in the object database.
// The "sha-like" condition ensures that refs like `master` are always
// freshened.
func AlreadyHaveRef(gitDir, sha string) bool {
	if !ShaLike.MatchString(sha) {
		return false
	}
	cmd := Command(gitDir, "git", "cat-file", "-t", sha)
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Run()
	return err == nil
}

// HaveFile checks if a git directory has files checked out.
func HaveFile(gitDir, ref, path string) (ok bool, err error) {
	cmd := Command(gitDir, "git", "show", fmt.Sprintf("%s:%s", ref, path))
	cmd.Stdout = nil // don't want to see the contents
	err = cmd.Run()
	ok = true
	if err != nil {
		ok = false
		if err.Error() == "exit status 128" {
			// This happens if the file doesn't exist.
			err = nil
		}
	}
	return ok, err
}

// RevParse parses and formats the git rev of a given git reference.
func RevParse(gitDir, ref string) (sha string, err error) {
	cmd := Command(gitDir, "git", "rev-parse", ref)
	cmd.Stdout = nil // for cmd.Output

	var stdout []byte
	stdout, err = cmd.Output()
	if err != nil {
		return "", err
	}

	sha = strings.TrimSpace(string(stdout))
	return sha, nil
}

// Describe describes a commit given a reference using the most recent tag
// reachable from it.
func Describe(gitDir, ref string) (desc string, err error) {
	cmd := Command(gitDir, "git", "describe", "--all", "--tags", "--long", ref)
	cmd.Stdout = nil // for cmd.Output

	var stdout []byte
	stdout, err = cmd.Output()
	if err != nil {
		return "", err
	}

	desc = strings.TrimSpace(string(stdout))
	desc = strings.TrimPrefix(desc, "heads/")
	return desc, nil
}

// Checkout switches branches or restores working tree files.
func Checkout(gitDir, checkoutDir, ref string) error {
	err := os.MkdirAll(checkoutDir, 0777)
	if err != nil {
		return err
	}

	args := []string{"--work-tree", checkoutDir, "checkout", ref, "--", "."}
	err = Command(gitDir, "git", args...).Run()
	if err != nil {
		return err
	}

	// Set mtimes to time file is most recently affected by a commit.
	// This is annoying but unfortunately git sets the timestamps to now,
	// and docker depends on the mtime for cache invalidation.
	err = SetMTimes(gitDir, checkoutDir, ref)
	if err != nil {
		return err
	}

	return nil
}

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
func PrepBuildDirectory(gitDir, remote, ref string) (*BuildDirectory, error) {
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

	err = LocalMirror(remote, gitDir, ref, os.Stderr)
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

	err = RecursiveCheckout(gitDir, checkoutPath, rev)
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

// RecursiveCheckout recursively checks out repositories; similar to "git clone
// --recursive".
func RecursiveCheckout(gitDir, checkoutPath, rev string) error {
	err := Checkout(gitDir, checkoutPath, rev)
	if err != nil {
		return fmt.Errorf("failed to checkout: %v", err)
	}

	err = PrepSubmodules(gitDir, checkoutPath, rev)
	if err != nil {
		return fmt.Errorf("failed to prep submodules: %v", err)
	}
	return nil
}

// SafeCleanup recursively removes all files from a given path, which has to be
// a subfolder of the current working directory.
func SafeCleanup(path string) error {
	if path == "/" || path == "" || path == "." || strings.Contains(path, "..") {
		return fmt.Errorf("invalid path specified for deletion %q", path)
	}
	return os.RemoveAll(path)
}
