// The resource package provides your charm access to Juju resources.
package resource

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"

	"github.com/juju/errors"
	"github.com/juju/gocharm/hook"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

type Service struct {
	ctx        *hook.Context
	state      localState
	r          *hook.Registry
	installers map[string]Installer
}

type localState struct {
	Hashes map[string]string
}

func (s *Service) setContext(ctx *hook.Context) error {
	s.ctx = ctx
	return nil
}

// Register registers the service with the given registry.
func (s *Service) Register(r *hook.Registry) {
	r.RegisterContext(s.setContext, &s.state)
	r.RegisterHook("install", s.updateResources)
	r.RegisterHook("upgrade-charm", s.updateResources)
	s.installers = make(map[string]Installer)
	s.r = r
}

type Installer func(resourcePath string) error

// Reg registers the resource name to the resources service. Given name and
// description are used to register the resource with the hook.Registry,
// the installers is called when the resource changes on charm deploy or upgrade.
// Each time the install or upgrade-charm hooks are called a hash is made of the,
// resource. When the hash for a resource is changed or non-existent the installers is called.
func (s *Service) Reg(name, description string, installer Installer) {
	s.r.RegisterResource(resource.Meta{
		Name:        name,
		Type:        resource.TypeFile,
		Path:        name,
		Description: description,
	})

	s.installers[name] = installer
}

// GetPath returns the local path to the file for a named resource.
func (s *Service) GetPath(name string) (string, error) {
	if b, e := s.ctx.Runner.Run("resource-get", name); e == nil {
		return string(b), nil
	} else {
		return "", errors.Annotatef(e, "resource-get of %s failed", name)
	}
}

// Has returns if given resource is available.
func (s *Service) Has(name string) bool {
	if p, e := s.GetPath(name); e == nil {
		return p != ""
	} else {
		return false
	}
}

func (s *Service) updateResources() error {
	if len(s.installers) == 0 {
		return nil
	}

	if s.state.Hashes == nil {
		s.state.Hashes = make(map[string]string)
	}

	for name, i := range s.installers {
		if s.Has(name) {
			if e := s.installOrUpdate(name, i); e != nil {
				return e
			}
		}
	}

	return nil
}

func (s *Service) installOrUpdate(name string, i Installer) error {
	if path, e := s.GetPath(name); e == nil {
		hash, e := makeHash(path)
		if e != nil {
			return errors.Annotatef(e, "creating a hash for %s failed", name)
		}

		if h, has := s.state.Hashes[name]; !(has && h == hash) {
			if e = errors.Annotatef(i(path), "installation of %s failed", name); e == nil {
				s.state.Hashes[name] = hash
			} else {
				return e
			}
		}
	} else {
		return e
	}

	return nil
}

func makeHash(path string) (string, error) {
	h := sha1.New()
	f, e := os.Open(path)
	if e != nil {
		return "", errors.Annotate(e, "os.Open failed")
	}

	defer f.Close()
	_, e = io.Copy(h, f)
	if e != nil {
		return "", errors.Annotate(e, "io.Copy failed")
	}

	return base64.RawURLEncoding.EncodeToString(h.Sum([]byte{})), nil
}
