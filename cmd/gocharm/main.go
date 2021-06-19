// Gocharm processes a Go package ("." by default) and builds it as a
// Juju charm. It should be invoked as follows:
//
//	gocharm [flags] [package]
//
// The following flags are supported:
//
//	  -repo="": charm repo directory (defaults to $JUJU_REPOSITORY)
//	  -v=false: print information about charms being built
//
// In order to qualify as a charm, a Go package must implement
// a RegisterHooks function with the following signature:
//
//	func RegisterHooks(r *hook.Registry)
//
// This function should register any resources required by the
// charm when it runs, including hooks, relations, configuration
// options. See the hook package (github.com/mever/gocharm/v2/hook)
// for an explanation of the hook registry.
//
// The charm is installed into the $JUJU_REPOSITORY/$name directory.
// $name is the last element of the package path. This directory is referred to as $charmdir below.
//
// If there is a file named README.md, a copy of it will be
// created in $charmdir.
//
// The charm binary will be installed into $charmdir/bin/runhook.
// A $charmdir/config.yaml file will be created containing
// all registered charm configuration options.
// A hooks directory will be created containing an entry
// for each registered hook.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/juju/charm/v9"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/utils/fs"
	"gopkg.in/errgo.v1"
)

var (
	repo    = flag.String("repo", "", "charm repo directory (defaults to $JUJU_REPOSITORY)")
	verbose = flag.Bool("v", false, "print information about charms being built")
	keep    = flag.Bool("keep", false, "do not delete temporary files")
)

func main() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "usage: gocharm [flags] [package]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if *repo == "" {
		if *repo = os.Getenv("JUJU_REPOSITORY"); *repo == "" {
			fatalf("JUJU_REPOSITORY environment variable not set")
		}
	}
	var pkgPath string
	switch flag.NArg() {
	case 0:
		pkgPath = "."
	case 1:
		pkgPath = flag.Arg(0)
	default:
		flag.Usage()
	}
	if err := main1(pkgPath); err != nil {
		fatalf("%v", err)
	}
}

func main1(pkgPath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return errgo.Notef(err, "cannot get current directory")
	}
	// Ensure that the package and all its dependencies are
	// installed before generating anything. This ensures
	// that we can generate the binary quickly, and that
	// it will be in sync with any package that have uninstalled
	// changes.
	if err := runCmd("", nil, "go", "install", pkgPath).Run(); err != nil {
		return errgo.Notef(err, "cannot install %q", pkgPath)
	}
	pkg, err := build.Default.Import(pkgPath, cwd, 0)
	if err != nil {
		return errgo.Notef(err, "cannot import %q", pkgPath)
	}
	charmName := path.Base(pkg.Dir)
	dest := filepath.Join(*repo, charmName)

	if _, err := canClean(dest); err != nil {
		return errgo.Notef(err, "cannot clean destination directory")
	}
	rev, err := readRevision(dest)
	if err != nil {
		return errgo.Notef(err, "cannot read revision")
	}

	// We put everything into a directory in /tmp first,
	// so we have less chance of deleting everything from
	// the destination without having something to replace
	// it with.
	tempDir, err := ioutil.TempDir("", "gocharm")
	if err != nil {
		return errgo.Notef(err, "cannot make temporary directory")
	}
	if !*keep {
		defer func() {_ = os.RemoveAll(tempDir)}()
	}

	tempCharmDir := filepath.Join(tempDir, "charm")
	if err := buildCharm(buildCharmParams{
		pkg:      pkg,
		charmDir: tempCharmDir,
		tempDir:  tempDir,
	}); err != nil {
		return errgo.Mask(err)
	}

	// The local revision number should not matter, but
	// there is a bug in juju that means that the charm
	// will not be correctly uploaded if it is not there, so we
	// preserve the revision found in the destination directory.
	if rev != -1 {
		rev++
		if err := writeRevision(tempCharmDir, rev); err != nil {
			return errgo.Notef(err, "cannot write revision file")
		}
	}
	if err := cleanDestination(dest); err != nil {
		return errgo.Mask(err)
	}
	if err := os.MkdirAll(dest, 0777); err != nil {
		return errgo.Mask(err)
	}
	for name := range allowed {
		from := filepath.Join(tempCharmDir, name)
		if _, err := os.Stat(from); err != nil {
			if !os.IsNotExist(err) {
				return errgo.Mask(err)
			}
			continue
		}
		if err := fs.Copy(from, filepath.Join(dest, name)); err != nil {
			return errgo.Notef(err, "cannot copy to final destination")
		}
	}
	curl := &charm.URL{
		Schema:   "local",
		Name:     charmName,
		Revision: -1,
	}
	fmt.Println(curl)
	return nil
}

func cleanDestination(dir string) error {
	needRemove, err := canClean(dir)
	if err != nil {
		return errgo.Mask(err)
	}
	for _, p := range needRemove {
		if *verbose {
			log.Printf("removing %s", p)
		}
		if err := os.RemoveAll(p); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

var allowed = map[string]bool{
	"assets":           true,
	"bin":              true,
	"compile":          true,
	"config.yaml":      true,
	"dependencies.tsv": true,
	"hooks":            true,
	"metadata.yaml":    true,
	"pkg":              true, // This allows us to test the compile scripts in the charm dir.
	"README.md":        true,
	"revision":         true,
	"src":              true,
}

func canClean(dir string) (needRemove []string, err error) {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errgo.Mask(err)
	}
	var toRemove []string
	for _, info := range infos {
		if info.Name()[0] == '.' {
			continue
		}
		if !allowed[info.Name()] {
			return nil, errgo.Newf("unexpected file %q found in %s", info.Name(), dir)
		}
		p := filepath.Join(dir, info.Name())
		if strings.HasSuffix(p, ".yaml") && !autogenerated(p) {
			return nil, errgo.Newf("non-autogenerated file %q", p)
		}
		toRemove = append(toRemove, p)
	}
	return toRemove, nil
}

func autogenerated(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, len(yamlAutogenComment))
	if _, err := io.ReadFull(f, buf); err != nil {
		return false
	}
	return bytes.Equal(buf, []byte(yamlAutogenComment))
}

func readRevision(charmDir string) (int, error) {
	p := revisionPath(charmDir)
	data, err := ioutil.ReadFile(p)
	if os.IsNotExist(err) {
		// No revision file, nothing to increment.
		return -1, nil
	}
	if err != nil {
		return 0, errgo.Mask(err)
	}
	rev, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || rev < 0 {
		return 0, fmt.Errorf("invalid number %q in %s", data, p)
	}
	return rev, nil
}

func writeRevision(charmDir string, rev int) error {
	return ioutil.WriteFile(revisionPath(charmDir), []byte(strconv.Itoa(rev)), 0666)
}

func revisionPath(charmDir string) string {
	return filepath.Join(charmDir, "revision")
}

func errorf(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "gocharm: %s\n", fmt.Sprintf(f, a...))
}

func fatalf(f string, a ...interface{}) {
	errorf(f, a...)
	os.Exit(2)
}
