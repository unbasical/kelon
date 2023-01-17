package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type ServiceID string

const (
	ServicePostgresSQL ServiceID = "pg"
	ServiceMySQL       ServiceID = "mysql"
	ServiceMongoDB     ServiceID = "mongo"
	ServiceSpiceDB     ServiceID = "spice"
	ServiceZed         ServiceID = "zed"
)

func serviceFromString(value string) (ServiceID, error) {
	switch value {
	case string(ServiceMongoDB):
		return ServiceMongoDB, nil
	case string(ServiceMySQL):
		return ServiceMySQL, nil
	case string(ServicePostgresSQL):
		return ServicePostgresSQL, nil
	case string(ServiceSpiceDB):
		return ServiceSpiceDB, nil
	case string(ServiceZed):
		return ServiceZed, nil
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

//nolint:gocyclo,gocritic
func (env *ContainerEnvironment) Start(ctx context.Context) error {
	if env.running {
		return nil
	}

	if !env.configured {
		return errors.Errorf("environment not configured")
	}

	//
	// Postgres
	if err := env.startPostgresSQL(ctx); err != nil {
		return err
	}

	//
	// MySQL
	if err := env.startMySQL(ctx); err != nil {
		return err
	}

	//
	// MongoDB
	if err := env.startMongoDB(ctx); err != nil {
		return err
	}

	//
	// SpiceDB
	if err := env.startSpiceDB(ctx); err != nil {
		return err
	}

	//
	// Zed - used for SpiceDB setup
	if err := env.startZed(ctx); err != nil {
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

//nolint:dupl,gocritic
func (env *ContainerEnvironment) startMySQL(ctx context.Context) error {
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
		mysql.WaitingFor = wait.ForAll(mysql.WaitingFor, strategy)
	}
	if ports, ok := env.portsToExpose[ServiceMySQL]; ok {
		mysql.ExposedPorts = ports
	}
	return env.startContainer(ctx, &mysql, ServiceMySQL)
}

//nolint:dupl,gocritic
func (env *ContainerEnvironment) startPostgresSQL(ctx context.Context) error {
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
	if strategy, ok := env.waitStrategies[ServicePostgresSQL]; ok {
		postgres.WaitingFor = wait.ForAll(postgres.WaitingFor, strategy)
	}
	if ports, ok := env.portsToExpose[ServicePostgresSQL]; ok {
		postgres.ExposedPorts = ports
	}

	return env.startContainer(ctx, &postgres, ServicePostgresSQL)
}

//nolint:dupl,gocritic
func (env *ContainerEnvironment) startMongoDB(ctx context.Context) error {
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
		mongo.WaitingFor = wait.ForAll(mongo.WaitingFor, strategy)
	}
	if ports, ok := env.portsToExpose[ServiceMongoDB]; ok {
		mongo.ExposedPorts = ports
	}

	return env.startContainer(ctx, &mongo, ServiceMongoDB)
}

//nolint:dupl,gocritic
func (env *ContainerEnvironment) startSpiceDB(ctx context.Context) error {
	spice := tc.ContainerRequest{
		Image: "authzed/spicedb:v1.16.0",
		Env: map[string]string{
			"SPICEDB_GRPC_PRESHARED_KEY": "spicykelon",
		},
		Entrypoint: []string{"spicedb", "serve"},
	}
	if strategy, ok := env.waitStrategies[ServiceSpiceDB]; ok {
		spice.WaitingFor = wait.ForAll(spice.WaitingFor, strategy)
	}
	if ports, ok := env.portsToExpose[ServiceSpiceDB]; ok {
		spice.ExposedPorts = ports
	}

	return env.startContainer(ctx, &spice, ServiceSpiceDB)
}

//nolint:dupl,gocritic
func (env *ContainerEnvironment) startZed(ctx context.Context) error {
	zedMntPath, err := filepath.Abs("../../examples/docker-compose/init/Init-SpiceDB.yml")
	if err != nil {
		return err
	}

	spicePort, err := env.Port(ctx, ServiceSpiceDB, "50051")
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("host.docker.internal:%s", spicePort)
	zed := tc.ContainerRequest{
		Image: "authzed/zed:v0.7.5",
		Mounts: []tc.ContainerMount{
			{
				Source:   tc.GenericBindMountSource{HostPath: zedMntPath},
				Target:   "/opt/Init-SpiceDB.yml",
				ReadOnly: false,
			},
		},
		Entrypoint: []string{
			"zed",
			"import",
			"file:///opt/Init-SpiceDB.yml",
			"--endpoint",
			endpoint,
			"--token",
			"spicykelon",
			"--insecure",
		},
	}
	if strategy, ok := env.waitStrategies[ServiceZed]; ok {
		zed.WaitingFor = wait.ForAll(zed.WaitingFor, strategy)
	}
	if ports, ok := env.portsToExpose[ServiceZed]; ok {
		zed.ExposedPorts = ports
	}

	if wErr := env.waitForSpiceDB(ctx); wErr != nil {
		return wErr
	}

	return env.startContainer(ctx, &zed, ServiceZed)
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

func (env *ContainerEnvironment) waitForSpiceDB(ctx context.Context) error {
	container, ok := env.container[ServiceSpiceDB]
	if !ok {
		return errors.New("no SpiceDB container found")
	}

	for i := 0; i < 10; i++ {
		_, reader, execErr := container.Exec(ctx, []string{"/usr/local/bin/grpc_health_probe", "-addr=localhost:50051"})
		if execErr != nil {
			return execErr
		}

		output := make([]byte, 1024)
		_, _ = reader.Read(output)
		if strings.Contains(string(output), "status: SERVING") {
			return nil
		}

		time.Sleep(5 * time.Second)
	}

	return errors.New("SpiceDB is not ready in time")
}
