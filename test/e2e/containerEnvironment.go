package e2e

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type ServiceID string

const (
	ServicePostgreSQL ServiceID = "pg"
	ServiceMySQL      ServiceID = "mysql"
	ServiceMongoDB    ServiceID = "mongo"
)

func serviceFromString(value string) (ServiceID, error) {
	switch value {
	case string(ServiceMongoDB):
		return ServiceMongoDB, nil
	case string(ServiceMySQL):
		return ServiceMySQL, nil
	case string(ServicePostgreSQL):
		return ServicePostgreSQL, nil
	default:
		return "", errors.Errorf("unknown service %s", value)
	}
}

type ContainerConfiguration struct {
	waitStrategies map[ServiceID]wait.Strategy
	exposePorts    map[ServiceID][]string
}

type ContainerEnvironment struct {
	configured     bool
	running        bool
	container      map[ServiceID]tc.Container
	waitStrategies map[ServiceID]wait.Strategy
	portsToExpose  map[ServiceID][]string
	exposedPorts   map[ServiceID]map[string]string
}

func newE2EEnvironment() *ContainerEnvironment {
	return &ContainerEnvironment{
		configured:     false,
		running:        false,
		container:      map[ServiceID]tc.Container{},
		waitStrategies: map[ServiceID]wait.Strategy{},
		portsToExpose:  map[ServiceID][]string{},
		exposedPorts:   map[ServiceID]map[string]string{},
	}
}

func (env *ContainerEnvironment) Configure(config ContainerConfiguration) {
	env.waitStrategies = config.waitStrategies
	env.portsToExpose = config.exposePorts

	env.configured = true
}

func (env *ContainerEnvironment) Start(ctx context.Context) error {
	if env.running {
		return nil
	}

	if !env.configured {
		return errors.Errorf("environment not configured")
	}

	postgresMntPath, err := filepath.Abs("../../examples/docker-compose/init/Init-Postgres.sql")
	if err != nil {
		return err
	}

	postgres := tc.ContainerRequest{
		Image: "docker.io/postgres:15",
		Env: map[string]string{
			"POSTGRES_DB":       "appstore",
			"POSTGRES_USER":     "You",
			"POSTGRES_PASSWORD": "SuperSecure",
		},
		Mounts: []tc.ContainerMount{
			{
				Source:   tc.GenericBindMountSource{HostPath: postgresMntPath},
				Target:   "/docker-entrypoint-initdb.d/Init-Postgres.sql",
				ReadOnly: false,
			},
		},
	}
	if strategy, ok := env.waitStrategies[ServicePostgreSQL]; ok {
		postgres.WaitingFor = strategy
	}
	if ports, ok := env.portsToExpose[ServicePostgreSQL]; ok {
		postgres.ExposedPorts = ports
	}

	if cErr := env.startContainer(ctx, &postgres, ServicePostgreSQL); err != nil {
		return cErr
	}

	mysqlMntPath, err := filepath.Abs("../../examples/docker-compose/init/Init-MySql.sql")
	if err != nil {
		return err
	}

	mysql := tc.ContainerRequest{
		Image: "docker.io/mysql:8",
		Env: map[string]string{
			"MYSQL_DATABASE":      "appstore",
			"MYSQL_USER":          "You",
			"MYSQL_PASSWORD":      "SuperSecure",
			"MYSQL_ROOT_PASSWORD": "root-beats-everything",
		},
		Mounts: []tc.ContainerMount{
			{
				Source:   tc.GenericBindMountSource{HostPath: mysqlMntPath},
				Target:   "/docker-entrypoint-initdb.d/Init-MySql.sql",
				ReadOnly: false,
			},
		},
	}
	if strategy, ok := env.waitStrategies[ServiceMySQL]; ok {
		mysql.WaitingFor = strategy
	}
	if ports, ok := env.portsToExpose[ServiceMySQL]; ok {
		mysql.ExposedPorts = ports
	}
	if cErr := env.startContainer(ctx, &mysql, ServiceMySQL); err != nil {
		return cErr
	}

	mongoMntPath, err := filepath.Abs("../../examples/docker-compose/init/Init-Mongo.js")
	if err != nil {
		return err
	}

	mongo := tc.ContainerRequest{
		Image: "docker.io/mongo:6",
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": "Root",
			"MONGO_INITDB_ROOT_PASSWORD": "RootPwd",
			"MONGO_INITDB_DATABASE":      "appstore",
		},
		Mounts: []tc.ContainerMount{
			{
				Source:   tc.GenericBindMountSource{HostPath: mongoMntPath},
				Target:   "/docker-entrypoint-initdb.d/init-mongo.js",
				ReadOnly: false,
			},
		},
	}
	if strategy, ok := env.waitStrategies[ServiceMongoDB]; ok {
		mongo.WaitingFor = strategy
	}
	if ports, ok := env.portsToExpose[ServiceMongoDB]; ok {
		mongo.ExposedPorts = ports
	}
	if err := env.startContainer(ctx, &mongo, ServiceMongoDB); err != nil {
		return err
	}

	env.running = true

	return nil
}

func (env *ContainerEnvironment) Stop(ctx context.Context) {
	if env.running {
		for _, container := range env.container {
			_ = container.Terminate(ctx)
		}

		env.running = false
	}
}

func (env *ContainerEnvironment) Host(ctx context.Context, service ServiceID) (string, error) {
	c, ok := env.container[service]
	if !ok {
		return "", errors.Errorf("unable to find service [%s]", service)
	}

	return c.Host(ctx)
}

func (env *ContainerEnvironment) Port(ctx context.Context, service ServiceID, port string) (string, error) {
	c, ok := env.container[service]
	if !ok {
		return "", errors.Errorf("unable to find service [%s]", service)
	}

	p, err := c.MappedPort(ctx, nat.Port(port))
	if err != nil {
		return "", err
	}

	return strings.Split(string(p), "/")[0], nil
}

func (env *ContainerEnvironment) startContainer(ctx context.Context, req *tc.ContainerRequest, name ServiceID) error {
	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: *req,
		Started:          true,
	})
	if err != nil {
		return err
	}

	env.container[name] = c

	for _, p := range env.portsToExpose[name] {
		mapped, pErr := env.Port(ctx, name, p)
		if pErr != nil {
			return pErr
		}

		if toExpose, ok := env.exposedPorts[name]; ok {
			toExpose[p] = mapped
		} else {
			env.exposedPorts[name] = map[string]string{
				p: mapped,
			}
		}
	}

	return nil
}
