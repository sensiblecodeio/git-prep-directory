package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LocalMirror creates or updates a mirror of `url` at `gitDir` using `git clone
// --mirror`.
func LocalMirror(url, gitDir, ref string, timeout time.Duration, messages io.Writer) error {
	ctx, done := context.WithTimeout(context.Background(), timeout)
	defer done()

	if _, err := os.Stat(gitDir); err == nil {
		// Repo already exists, don't need to clone it.

		if AlreadyHaveRef(gitDir, ref) {
			// Sha already exists, don't need to fetch.
			// fmt.Fprintf(messages, "Already have ref: %v %v", gitDir, ref)
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
	desc = strings.TrimPrefix(desc, "tags/")
	return desc, nil
}

// RecursiveCheckout recursively checks out repositories; similar to "git clone
// --recursive".
func RecursiveCheckout(gitDir, checkoutPath, rev string, timeout time.Duration, messages io.Writer) error {
	err := Checkout(gitDir, checkoutPath, rev)
	if err != nil {
		return fmt.Errorf("failed to checkout: %v", err)
	}

	err = PrepSubmodules(gitDir, checkoutPath, rev, timeout, messages)
	if err != nil {
		return fmt.Errorf("failed to prep submodules: %v", err)
	}
	return nil
}

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
