package git

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	ini "github.com/vaughan0/go-ini"
)

// PrepSubmodules in parallel initializes all submodules and additionally stores
// them in a local cache.
func PrepSubmodules(gitDir, checkoutDir, mainRev string, timeout time.Duration, messages io.Writer) error {
	gitModules := filepath.Join(checkoutDir, ".gitmodules")

	submodules, err := ParseSubmodules(gitModules)
	if err != nil {
		if os.IsNotExist(err) {
			// No .gitmodules available.
			return nil
		}
		return err
	}

	log.Printf("Prep %v submodules", len(submodules))

	if err := GetSubmoduleRevs(gitDir, mainRev, submodules); err != nil {
		return fmt.Errorf("GetSubmoduleRevs: %v", err)
	}

	errs := make(chan error, len(submodules))

	go func() {
		defer close(errs)

		var wg sync.WaitGroup
		defer wg.Wait()

		// Run only NumCPU in parallel
		semaphore := make(chan struct{}, runtime.NumCPU())

		for _, submodule := range submodules {

			wg.Add(1)
			go func(submodule Submodule) {
				defer wg.Done()
				defer func() { <-semaphore }()
				semaphore <- struct{}{}

				err := prepSubmodule(gitDir, checkoutDir, submodule, timeout, messages)
				if err != nil {
					err = fmt.Errorf("processing %v: %v", submodule.Path, err)
				}
				errs <- err
			}(submodule)
		}
	}()

	// errs chan has buffer length len(submodules)
	err = MultipleErrors(errs)
	if err != nil {
		return err
	}
	return nil
}

// ErrMultiple holds a list of errors.
type ErrMultiple struct {
	errs []error
}

func (em *ErrMultiple) Error() string {
	var s []string
	for _, e := range em.errs {
		s = append(s, e.Error())
	}
	return fmt.Sprint("multiple errors:\n", strings.Join(s, "\n"))
}

// MultipleErrors reads errors out of a channel, counting only the non-nil ones.
// If there are zero non-nil errs, nil is returned.
func MultipleErrors(errs <-chan error) error {
	var em ErrMultiple
	for e := range errs {
		if e == nil {
			continue
		}
		em.errs = append(em.errs, e)
	}
	if len(em.errs) == 0 {
		return nil
	}
	return &em
}

// Checkout the working directory of a given submodule.
func prepSubmodule(mainGitDir, mainCheckoutDir string, submodule Submodule, timeout time.Duration, messages io.Writer) error {
	subGitDir := filepath.Join(mainGitDir, "modules", submodule.Path)

	err := LocalMirror(submodule.URL, subGitDir, submodule.Rev, timeout, messages)
	if err != nil {
		return err
	}

	subCheckoutPath := filepath.Join(mainCheckoutDir, submodule.Path)

	// Note: checkout may recurse onto prepSubmodules.
	err = RecursiveCheckout(subGitDir, subCheckoutPath, submodule.Rev, timeout, messages)
	if err != nil {
		return err
	}
	return err
}

// Submodule holds the path, url, and revision to a submodule.
type Submodule struct {
	Path string
	URL  string
	Rev  string // populated by GetSubmoduleRevs
}

// ParseSubmodules returns all submodule definitions given a .gitmodules
// configuration.
func ParseSubmodules(filename string) ([]Submodule, error) {
	config, err := ini.LoadFile(filename)
	if err != nil {
		return nil, err
	}

	var submodules []Submodule
	for section := range config {
		if !strings.HasPrefix(section, "submodule") {
			continue
		}
		submodules = append(submodules, Submodule{
			Path: config.Section(section)["path"],
			URL:  config.Section(section)["url"],
		})
	}
	return submodules, nil
}

// GetSubmoduleRevs returns the revisions of all files in a given list of
// submodules.
func GetSubmoduleRevs(gitDir, mainRev string, submodules []Submodule) error {
	for i := range submodules {
		rev, err := GetSubmoduleRev(gitDir, submodules[i].Path, mainRev)
		if err != nil {
			return err
		}
		submodules[i].Rev = rev
	}
	return nil
}

// GetSubmoduleRev returns the revisions of all files in a given submodule.
func GetSubmoduleRev(gitDir, submodulePath, mainRev string) (string, error) {
	cmd := Command(gitDir, "git", "ls-tree", mainRev, "--", submodulePath)
	cmd.Stdout = nil

	parts, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.Fields(string(parts))[2], nil
}
