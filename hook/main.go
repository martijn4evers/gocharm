package hook

import (
	"encoding/json"
	"github.com/juju/charm/v9/hooks"
	"log"
	"os"
	"sort"
	"strings"

	"gopkg.in/errgo.v1"
)

const (
	envUUID          = "JUJU_MODEL_UUID"
	envUnitName      = "JUJU_UNIT_NAME"
	envCharmDir      = "CHARM_DIR"
	envJujuContextId = "JUJU_CONTEXT_ID"
	envRelationName  = "JUJU_RELATION"
	envRelationId    = "JUJU_RELATION_ID"
	envRemoteUnit    = "JUJU_REMOTE_UNIT"
	envSocketPath    = "JUJU_AGENT_SOCKET"
)

var mustEnvVars = []string{
	envUUID,
	envUnitName,
	envCharmDir,
	envJujuContextId,
}

var relationEnvVars = []string{
	envRelationName,
	envRelationId,
	// Note that envRemoteUnit is not guaranteed
	// to be set for relation-broken hooks.
}

// Main creates a new context from the environment and invokes the
// appropriate command or hook functions from the given
// registry or sub-registries of it.
//
// If a long-lived command is started, Main returns it rather
// than waiting for it to complete. This makes it possible
// to run command functions in tests.
//
// The ctxt value holds the context that will be passed
// to the hooks; the state value is used to retrieve
// and save persistent state.
//
// This function is designed to be called by gocharm
// generated code and tests only.
func Main(r *Registry, ctxt *Context, state PersistentState) (_ Command, err error) {
	if ctxt.RunCommandName != "" {
		log.Printf("running command %q %q", ctxt.RunCommandName, ctxt.RunCommandArgs)
		cmd := r.commands[ctxt.RunCommandName]
		if cmd == nil {
			return nil, usageError(r)
		}
		return cmd(ctxt.RunCommandArgs)
	}
	ctxt.Logf("running hook %s {", ctxt.HookName)
	defer ctxt.Logf("} %s", ctxt.HookName)
	// Retrieve all persistent state.
	// TODO read all of the state in one operation from a single file?
	if err := loadState(r, state); err != nil {
		return nil, errgo.Mask(err)
	}
	// Notify everyone about the context.
	for _, setter := range r.contexts {
		if err := setter(ctxt); err != nil {
			return nil, errgo.Notef(err, "cannot set context")
		}
	}
	defer func() {
		// All the hooks have now run; save the state.
		saveErr := saveState(r, state)
		if saveErr == nil {
			return
		}
		if err == nil {
			err = errgo.Notef(saveErr, "cannot save local state")
			return
		}
		ctxt.Logf("cannot save local state: %v", saveErr)
	}()

	// The wildcard hook always runs after any other
	// registered hooks.
	hookFuncs := r.hooks[ctxt.HookName]

	if len(hookFuncs) == 0 {
		ctxt.Logf("hook %q not registered", ctxt.HookName)
		return nil, usageError(r)
	}
	hookFuncs = append(hookFuncs, r.hooks["*"]...)
	for _, f := range hookFuncs {
		if err := f.run(); err != nil {
			// TODO better error context here, perhaps
			// including local state name, hook name, etc.
			return nil, errgo.Mask(err)
		}
	}
	return nil, nil
}

func loadState(r *Registry, state PersistentState) error {
	for _, val := range r.state {
		data, err := state.Load(val.registryName)
		if err != nil {
			return errgo.Notef(err, "cannot load state for %s", val.registryName)
		}
		if data == nil {
			continue
		}
		if err := json.Unmarshal(data, val.val); err != nil {
			return errgo.Notef(err, "cannot unmarshal state for %s", val.registryName)
		}
	}
	return nil
}

func saveState(r *Registry, state PersistentState) (err error) {
	for _, val := range r.state {
		data, err := json.Marshal(val.val)
		if err != nil {
			return errgo.Notef(err, "cannot marshal state for %s", val.registryName)
		}
		if err := state.Save(val.registryName, data); err != nil {
			return errgo.Notef(err, "cannot save state for %s", val.registryName)
		}
	}
	return nil
}

func usageError(r *Registry) error {
	var allowed []string
	for cmd := range r.commands {
		allowed = append(allowed, "cmd-"+cmd+" [arg...]")
	}
	for hook := range r.hooks {
		allowed = append(allowed, hook)
	}
	sort.Strings(allowed[0:len(r.commands)])
	sort.Strings(allowed[len(r.commands):])
	return errgo.Newf("usage: runhook %s", strings.Join(allowed, "\n\t| runhook "))
}

func nop() error {
	return nil
}

// RegisterMainHooks registers any hooks that
// are needed by any charm. It should be
// called after any other Register functions.
//
// This function is designed to be called by gocharm
// generated code only.
func RegisterMainHooks(r *Registry) {
	// We always need install and start hooks.
	r.RegisterHook("install", nop)
	r.RegisterHook("start", nop)
	// TODO Perhaps... ensure that we have a stop hook, and make
	// it clean up our persistent state. But that may not be
	// right if "stop" is considered something we can start
	// from again.
}

// NewContextFromEnvironment creates a hook context from the current
// environment, using the given tool runner to acquire information to
// populate the context, and the given registry to determine which
// relations to fetch information for.
//
// The hookName argument holds the name of the hook
// to invoke, and args holds any additional arguments.
//
// The given directory will be used to save persistent state.
//
// It also returns the persistent state associated with the context
// unless called in a command-running context.
//
// The caller is responsible for calling Close on the returned
// context.
func NewContextFromEnvironment(r *Registry, stateDir string, hookName string, args []string) (*Context, PersistentState, error) {
	if hookName == "" {
		return nil, nil, errgo.Newf("no hook name provided")
	}
	if strings.HasPrefix(hookName, "cmd-") {
		return &Context{
			RunCommandName: strings.TrimPrefix(hookName, "cmd-"),
			RunCommandArgs: args,
		}, nil, nil
	}
	if len(args) != 0 {
		return nil, nil, errgo.Newf("unexpected extra arguments running hook %q: %v", hookName, args)
	}
	vars := mustEnvVars
	if os.Getenv(envRelationName) != "" {
		vars = append(vars, relationEnvVars...)
		if !strings.HasSuffix(hookName, "-"+string(hooks.RelationBroken)) {
			vars = append(vars, envRemoteUnit)
		}
	}
	for _, v := range vars {
		if os.Getenv(v) == "" {
			return nil, nil, errgo.Newf("required environment variable %q not set", v)
		}
	}
	runner, err := newToolRunnerFromEnvironment()
	if err != nil {
		return nil, nil, errgo.Notef(err, "cannot make runner")
	}
	ctxt := &Context{
		UUID:         os.Getenv(envUUID),
		Unit:         UnitId(os.Getenv(envUnitName)),
		CharmDir:     os.Getenv(envCharmDir),
		RelationName: os.Getenv(envRelationName),
		RelationId:   RelationId(os.Getenv(envRelationId)),
		RemoteUnit:   UnitId(os.Getenv(envRemoteUnit)),
		HookName:     hookName,
		Runner:       runner,
		HookStateDir: stateDir,
	}

	// Populate the relation fields of the ContextInfo
	ctxt.RelationIds = make(map[string][]RelationId)
	ctxt.Relations = make(map[RelationId]map[UnitId]map[string]string)
	for name := range r.RegisteredRelations() {
		ids, err := ctxt.relationIds(name)
		if err != nil {
			return nil, nil, errgo.Notef(err, "cannot get relation ids for relation %q", name)
		}
		ctxt.RelationIds[name] = ids
		for _, id := range ids {
			units := make(map[UnitId]map[string]string)
			unitIds, err := ctxt.relationUnits(id)
			if err != nil {
				return nil, nil, errgo.Notef(err, "cannot get unit ids for relation id %q", id)
			}
			for _, unitId := range unitIds {
				settings, err := ctxt.getAllRelationUnit(id, unitId)
				if err != nil {
					return nil, nil, errgo.Notef(err, "cannot get settings for relation %s, unit %s", id, unitId)
				}
				units[unitId] = settings
			}
			ctxt.Relations[id] = units
		}
	}
	return ctxt, NewDiskState(ctxt.StateDir()), nil
}
