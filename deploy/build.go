package deploy

import (
	"bytes"
	"github.com/juju/charm/v9"
	"github.com/juju/charm/v9/resource"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/mever/gocharm/hook"

	"gopkg.in/errgo.v1"
	"gopkg.in/yaml.v2"
)

const (
	autogenMessage = `This file is automatically generated. Do not edit.`
	godepPath      = `github.com/tools/godep`
)

// BuildCharmParams holds parameters for the BuildCharm
// function.
type BuildCharmParams struct {
	// Registry holds the hook registry that
	// represents the charm to be built.
	Registry *hook.Registry

	// CharmDir specifies the destination directory to write
	// the charm files to. This will be created by
	// BuildCharm if needed.
	CharmDir string

	// Source specifies that the source code is included
	// in the charm and the charm executable will be
	// built from that at hook execution time.
	// The caller is responsible for creating an
	// executable named "compile" in the charm's
	// root directory which should build the hook executable
	// to "bin/runhook". This can be done after
	// calling BuildCharm.
	Source bool

	// HookBinary holds the path to the hook
	// executable (mutually exclusive to Source).
	HookBinary string

	// NoCompress specifies that the binary should
	// not be compressed in the charm.
	NoCompress bool
}

type charmBuilder BuildCharmParams

// BuildCharm builds a charm from the data
// registered in p.Registry and puts the
// result into p.CharmDir.
func BuildCharm(p BuildCharmParams) error {
	b := (*charmBuilder)(&p)
	if p.CharmDir == "" {
		return errgo.Newf("no charm directory provided")
	}
	if !p.Source {
		if p.HookBinary == "" {
			return errgo.Newf("no hook binary provided")
		}
	}
	r := b.Registry
	if err := b.writeHooks(r.RegisteredHooks()); err != nil {
		return errgo.Notef(err, "cannot write hooks to charm")
	}
	if err := b.writeMeta(r.RegisteredRelations(), r.RegisteredResources()); err != nil {
		return errgo.Notef(err, "cannot write metadata.yaml")
	}
	if err := b.writeConfig(r.RegisteredConfig()); err != nil {
		return errgo.Notef(err, "cannot write config.yaml")
	}
	if p.HookBinary != "" {
		if err := b.writeBinary(p.HookBinary); err != nil {
			return errgo.Notef(err, "cannot write hook binary")
		}
		if !b.NoCompress {
			if err := b.writeUncompressor(); err != nil {
				return errgo.Notef(err, "cannot write uncompressor script")
			}
		}
	}
	// Sanity check that the new config files parse correctly.
	if _, err := charm.ReadCharmDir(b.CharmDir); err != nil {
		return errgo.Notef(err, "charm will not read correctly; we've broken it, sorry")
	}
	return nil
}

// writeHooks ensures that the charm has the given set of hooks.
// TODO write install and start hooks even if they're not registered,
// because otherwise it won't be treated as a valid charm.
func (b *charmBuilder) writeHooks(hooks []string) error {
	hookDir := filepath.Join(b.CharmDir, "hooks")
	if err := os.MkdirAll(hookDir, 0777); err != nil {
		return errgo.Notef(err, "failed to make hooks directory")
	}
	// Add any new hooks we need to the charm directory.
	for _, hookName := range hooks {
		hookPath := filepath.Join(hookDir, hookName)
		if err := ioutil.WriteFile(hookPath, b.hookStub(hookName), 0755); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

// hookStubTemplate holds the template for the generated hook code.
// The apt-get flags are stolen from github.com/juju/utils/apt
var hookStubTemplate = template.Must(template.New("").Parse(`#!/bin/sh
set -ex
{{if .Source}}
{{if eq .HookName "install"}}
apt-get '--option=Dpkg::Options::=--force-confold'  '--option=Dpkg::options::=--force-unsafe-io' --assume-yes --quiet install golang git mercurial

if test -e "$CHARM_DIR/bin/runhook"
then
	# the binary has been pre-compiled; no need to compile again.
	exit 0
fi
export GOPATH="$CHARM_DIR"
go get {{.GodepPath}}

"$CHARM_DIR/compile"
{{else}}
if test -e "$CHARM_DIR/compile-always"
then
	"$CHARM_DIR/compile"
fi
{{end}}
{{else if not .NoCompress }}
"$CHARM_DIR/uncompress"
{{end}}
$CHARM_DIR/bin/runhook -run-hook {{.HookName}}
`))

func (b *charmBuilder) writeUncompressor() error {
	return ioutil.WriteFile(filepath.Join(b.CharmDir, "uncompress"), []byte(uncompressScript), 0777)
}

const uncompressScript = `#!/bin/sh
EXE="$CHARM_DIR/bin/runhook"
EXExz="$EXE.xz"
if test -e "$EXExz" -a '(' ! -e "$EXE" -o "$EXExz" -nt "$EXE" ')'
then
	echo uncompressing hook executable
	# the old binary might still be running, so move
	# it out of the way rather than overwriting it.
	mv "$EXE" "$EXE.old"
	xzcat "$EXExz" > "$EXE" || {
		echo cannot uncompress "$EXE" >&2
		exit 1
	}
	chmod 755 "$EXE"
fi
`

type hookStubParams struct {
	Source     bool
	HookName   string
	GodepPath  string
	NoCompress bool
}

func (b *charmBuilder) hookStub(hookName string) []byte {
	return executeTemplate(hookStubTemplate, hookStubParams{
		Source:     b.Source,
		HookName:   hookName,
		GodepPath:  godepPath,
		NoCompress: b.NoCompress,
	})
}

func (b *charmBuilder) writeMeta(relations map[string]charm.Relation, resources map[string]resource.Meta) error {
	var meta charm.Meta
	info := b.Registry.CharmInfo()
	meta.Name = info.Name
	meta.Summary = info.Summary
	meta.Description = info.Description
	meta.Provides = make(map[string]charm.Relation)
	meta.Requires = make(map[string]charm.Relation)
	meta.Peers = make(map[string]charm.Relation)
	meta.Resources = resources

	for name, rel := range relations {
		switch rel.Role {
		case charm.RoleProvider:
			meta.Provides[name] = rel
		case charm.RoleRequirer:
			meta.Requires[name] = rel
		case charm.RolePeer:
			meta.Peers[name] = rel
		default:
			return errgo.Newf("unknown role %q in relation", rel.Role)
		}
	}
	if err := writeYAML(filepath.Join(b.CharmDir, "metadata.yaml"), &meta); err != nil {
		return errgo.Notef(err, "cannot write metadata.yaml")
	}
	return nil
}

func (b *charmBuilder) writeConfig(config map[string]charm.Option) error {
	configPath := filepath.Join(b.CharmDir, "config.yaml")
	if len(config) == 0 {
		return nil
	}
	if err := writeYAML(configPath, &charm.Config{
		Options: config,
	}); err != nil {
		return errgo.Notef(err, "cannot write config.yaml")
	}
	return nil
}

func (b *charmBuilder) writeBinary(exe string) error {
	// TODO compress
	f, err := os.Open(exe)
	if err != nil {
		return errgo.Mask(err)
	}
	defer f.Close()
	binDir := filepath.Join(b.CharmDir, "bin")
	if err := os.MkdirAll(binDir, 0777); err != nil {
		return errgo.Notef(err, "failed to make hooks directory")
	}
	name := "runhook"
	mode := os.FileMode(0777)
	if !b.NoCompress {
		name += ".xz"
		mode = 0666
	}
	out, err := os.OpenFile(filepath.Join(binDir, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return errgo.Mask(err)
	}
	defer out.Close()
	if b.NoCompress {
		if _, err := io.Copy(out, f); err != nil {
			return errgo.Notef(err, "cannot copy binary")
		}
		return nil
	}
	xzCommand := exec.Command("xz")
	xzCommand.Stdout = out
	xzCommand.Stdin = f
	xzCommand.Stderr = os.Stderr
	if err := xzCommand.Run(); err != nil {
		return errgo.Notef(err, "xz compress failed")
	}
	return nil
}

const yamlAutogenComment = "# " + autogenMessage + "\n"

func writeYAML(file string, val interface{}) error {
	data, err := yaml.Marshal(val)
	if err != nil {
		return errgo.Notef(err, "cannot marshal YAML")
	}
	data = append([]byte(yamlAutogenComment), data...)
	if err := ioutil.WriteFile(file, data, 0666); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func executeTemplate(t *template.Template, param interface{}) []byte {
	var w bytes.Buffer
	if err := t.Execute(&w, param); err != nil {
		panic(err)
	}
	return w.Bytes()
}
