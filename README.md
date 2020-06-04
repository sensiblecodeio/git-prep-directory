# git-prep-directory

Build tools friendly way of repeatedly cloning a git repository using a
submodule cache and keeping timestamps to commit times.

## Concept

Executing the tool/library repeatedly on the same repository with different
commit refs results in two optimizations:

- Faster clone times due to less network traffic as most submodules change so
  infrequently that they are likely already exist in the submodule cache.
- Faster build times for tools like Make and Docker as unchanged files keep
  their timestamp this are cached by the build tools.

For each execution of a given commit ref the tool/library does the follow steps:

1. Clone the given repository
2. Checkout the given revision
3. Find all submodules and check if they are already cached
  - If cached, assert the required commit is checked out and initialize them
  - If not cached, checkout the submodule and store it in the cache.
4. Set the timestamp of all files and directories to their respective commit time.

This results in the following path hierarchy:

    .
    └── src
        ├── HEAD
        ├── c                       <-- repository commits
        │   ├── 39529a7ed3
        │   ├── 5e90e55e6b
        │   ├── 607f0489ec
        │   └── :
        ├── config
        ├── index
        ├── modules                 <-- submodules cache
        │   ├── path/to/submodule/1
        │   ├── path/to/submodule/2
        │   ├── path/to/submodule/3
        │   └── :
        ├── objects
        ├── packed-refs
        └── refs


## Command line tool

### Installation

    $ go get github.com/sensiblecodeio/git-prep-directory/cmd/git-prep-directory


### Usage

    $ git-prep-directory --help
    NAME:
       git-prep-directory - Build tools friendly way of repeatedly cloning a git
       repository using a submodule cache and setting file timestamps to commit times.

    USAGE:
       git-prep-directory [global options] command [command options] [arguments...]

    VERSION:
       1.0.0

    COMMANDS:
       help, h      Shows a list of commands or help for one command

    GLOBAL OPTIONS:
       --url, -u                    URL to clone
       --ref, -r                    ref to checkout
       --destination, -d "./src"    destination dir
       --help, -h                   show help
       --version, -v                print the version


## Go Library

```go
import "github.com/scraperwiki/git-prep-directory"

buildDirectory, err := git.PrepBuildDirectory(<OUT_PATH>, <REPO_URL>, <GIT_REF>)
```
