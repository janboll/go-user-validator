package example

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/app-sre/go-qontract-reconcile/pkg/reconcile"
	"github.com/app-sre/go-qontract-reconcile/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var EXAMPLE_INTEGRATION_NAME = "example"

type ListDirectoryFunc func(string) ([]os.FileInfo, error)
type ReadFileFunc func(string) ([]byte, error)
type GetUsers func(context.Context) (*UsersResponse, error)

type ExampleConfig struct {
	Tempdir string
}

func newExampleConfig() *ExampleConfig {
	var ec ExampleConfig
	sub := util.EnsureViperSub(viper.GetViper(), "example")
	sub.SetDefault("tempdir", "/tmp/example")
	sub.BindEnv("tempdir", "EXAMPLE_TEMPDIR")
	if err := sub.Unmarshal(&ec); err != nil {
		util.Log().Fatalw("Error while unmarshalling configuration %s", err.Error())
	}
	return &ec
}

type Example struct {
	config *ExampleConfig

	listDirectoryFunc ListDirectoryFunc
	readFileFunc      ReadFileFunc
	getUsersFunc      GetUsers
}

func NewExample() *Example {
	ec := newExampleConfig()
	return &Example{
		config: ec,
		listDirectoryFunc: func(path string) ([]os.FileInfo, error) {
			return ioutil.ReadDir(path)
		},
		readFileFunc: func(path string) ([]byte, error) {
			return ioutil.ReadFile(path)
		},
		getUsersFunc: func(ctx context.Context) (*UsersResponse, error) {
			return Users(ctx)
		},
	}
}

type UserFiles struct {
	FileNames string
	GpgKey    string
}

func (e *Example) CurrentState(ctx context.Context, ri *reconcile.ResourceInventory) error {
	util.Log().Infow("Getting current state")

	files, err := e.listDirectoryFunc(e.config.Tempdir)
	if err != nil {
		return errors.Wrap(err, "Error while reading workdir")
	}

	for _, f := range files {
		absolutePath := e.config.Tempdir + "/" + f.Name()
		util.Log().Debugw("Found file", "file", absolutePath)
		content, err := e.readFileFunc(absolutePath)
		if err != nil {
			return errors.Wrap(err, "Error while reading file")
		}
		rs := &reconcile.ResourceState{
			Current: &UserFiles{
				FileNames: f.Name(),
				GpgKey:    string(content),
			},
		}
		ri.AddResourceState(f.Name(), rs)
	}

	return nil
}

func (e *Example) DesiredState(ctx context.Context, ri *reconcile.ResourceInventory) error {
	util.Log().Infow("Getting desired state")

	users, err := e.getUsersFunc(ctx)

	if err != nil {
		return errors.Wrap(err, "Error while getting users")
	}

	for _, user := range users.GetUsers_v1() {
		state := ri.GetResourceState(user.GetOrg_username())
		if state == nil {
			state = &reconcile.ResourceState{}
			ri.AddResourceState(user.GetOrg_username(), state)
		}
		state.Config = user
		state.Desired = &UserFiles{
			FileNames: user.GetOrg_username(),
			GpgKey:    user.GetPublic_gpg_key(),
		}
	}

	return nil
}

func (e *Example) Reconcile(ctx context.Context, ri *reconcile.ResourceInventory) error {
	util.Log().Infow("Reconciling")

	for _, state := range ri.State {
		var current, desired *UserFiles
		if state.Current != nil {
			current = state.Current.(*UserFiles)
		}
		if state.Desired != nil {
			desired = state.Desired.(*UserFiles)
		}

		if current != nil && desired == nil {
			absolutePath := e.config.Tempdir + "/" + current.FileNames
			util.Log().Infow("Deleting file", "file", current.FileNames)
			err := os.Remove(absolutePath)
			if err != nil {
				return errors.Wrap(err, "Error while deleting file")
			}
		} else if current == nil || current.GpgKey != desired.GpgKey {
			absolutePath := e.config.Tempdir + "/" + desired.FileNames
			util.Log().Infow("Writing file", "file", desired.FileNames)
			err := ioutil.WriteFile(absolutePath, []byte(desired.GpgKey), 0644)
			if err != nil {
				return errors.Wrap(err, "Error while writing file")
			}
		}

	}
	return nil
}

func (e *Example) LogDiff(ri *reconcile.ResourceInventory) {
	util.Log().Debugw("Logging diff")

	for _, state := range ri.State {
		var current, desired *UserFiles
		if state.Current != nil {
			current = state.Current.(*UserFiles)
		}
		if state.Desired != nil {
			desired = state.Desired.(*UserFiles)
		}
		if current != nil && desired == nil {
			util.Log().Infow("Deleting", "file", current.FileNames)
		} else if current == nil || current.GpgKey != desired.GpgKey {
			util.Log().Infow("Updating", "file", desired.FileNames)
		}
	}
}

func (e *Example) Setup(context.Context) error {
	util.Log().Infow("Setting up example integration")
	err := os.MkdirAll(e.config.Tempdir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "Error while creating workdir")
	}

	return nil
}
