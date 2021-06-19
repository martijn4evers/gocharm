package main

import (
	"bytes"
	"encoding/json"
	"github.com/juju/charm/v9"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"gopkg.in/errgo.v1"
)

func registeredCharmInfo(pkg, tempDir string) (*charmInfo, error) {
	code := generateCode(inspectCode, pkg)
	goFile := filepath.Join(tempDir, "inspect.go")
	if err := ioutil.WriteFile(goFile, code, 0666); err != nil {
		return nil, errgo.Notef(err, "cannot write hook inspection code")
	}

	inspectExe := filepath.Join(tempDir, "inspect")
	if err := runCmd("",nil, "go", "build", "-o", inspectExe, goFile).Run(); err != nil {
		return nil, errgo.Notef(err, "cannot build hook inspection code")
	}

	c := exec.Command(inspectExe)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return nil, errgo.Notef(err, "failed to run inspect")
	}
	var out charmInfo
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal %q", err)
	}
	if len(out.Hooks) == 0 {
		return nil, errgo.New("no hooks registered")
	}
	if *verbose {
		log.Printf("registered hooks: %v", out.Hooks)
		log.Printf("%d registered relations", len(out.Meta.Requires)+len(out.Meta.Provides)+len(out.Meta.Peers))
		log.Printf("%d registered config options", len(out.Config))
	}

	return &out, nil
}

// charmInfo holds the information we glean
// from inspecting the hook registry.
// Note that this must be kept in sync with the
// version in inspectCode below.
type charmInfo struct {
	Hooks  []string
	Config map[string]charm.Option
	Meta   charm.Meta
}

var inspectCode = template.Must(template.New("").Parse(`
// {{.AutogenMessage}}

package main

import (
	"encoding/json"
	"github.com/juju/charm/v9"
	"fmt"
	"os"

	inspect {{.CharmPackage | printf "%q"}}
	{{.HookPackage | printf "%q"}}
)

// charmInfo must be kept in sync with the charmInfo
// type above.
type charmInfo struct {
	Hooks  []string
	Config map[string]charm.Option
	Meta   charm.Meta
}

func main() {
	r := hook.NewRegistry()
	inspect.RegisterHooks(r)
	hook.RegisterMainHooks(r)
	info := charmInfo{
		Hooks:	   r.RegisteredHooks(),
		Config:	   r.RegisteredConfig(),
	}

	info.Meta.Summary = r.CharmInfo().Summary
	info.Meta.Description = r.CharmInfo().Description
	info.Meta.Resources = r.RegisteredResources()
	info.Meta.Provides = make(map[string]charm.Relation)
	info.Meta.Requires = make(map[string]charm.Relation)
	for name, rel := range r.RegisteredRelations() {
		switch rel.Role {
		case charm.RoleProvider:
			info.Meta.Provides[name] = rel
		case charm.RoleRequirer:
			info.Meta.Requires[name] = rel
		case charm.RolePeer:
			info.Meta.Peers[name] = rel
		default:
			panic(fmt.Sprintf("unknown role %q in relation", rel.Role))
		}
	}

	data, err := json.Marshal(info)
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(data)
}
`))
