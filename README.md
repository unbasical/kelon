# Kelon

Kelon is a policy enforcement point, that is wrapping the [Open Policy Agent](https://www.openpolicyagent.org) (OPA) and adding more functionality in terms of microservices.

## Problems you face when applying authorizing to your services

Let's say you have some services you would like to have authorization implemented into. With this challenge in mind there are basically two options:

1. Use code to secure your endpoints 
    * In case of REST-Services nearly every framework provides something like Guards or so
2. Use tools to secure your endpoints 
    * Just add some small code snippets to your service (like Request-Interceptors) and let a tool handle the decision for you

It's obvious that the second option not only increases the speed at which you'll implement your service (**focusing only on the functionality**), but also grants much more
security because **all** your **policies** are **stored and enforced in one place** (they can also be separately tested).

This is basically the problem that the Open Policy Agent solves. The only problem is that it is very hard to integrate
the OPA into a project because it needs the data which is needed to enforce policies to be stored inside it. This leads to work flows where
you copy an abstraction of your entire database into OPA which is not only redundant, but also leads to synchronization issues.

## How Kelon solves authorization

Kelon is basically a proxy of OPA's Data-API which is connected to all your data sources and responds to incoming queries with "ALLOW" or "DENY".
This request contains all information about i.e. the incoming client request to your service.
Internally, Kelon uses the provided input to determine a [OPA-Package](https://www.openpolicyagent.org/docs/latest/policy-language/#packages) which it then sends a query to (using OPA's [Partial Evaluation](https://www.openpolicyagent.org/docs/latest/rest-api/#compile-api)).
The result of this query is interpreted and (in case of any "unknowns") translated into a data source query which will be used to make the decision.

## Getting Started

To run Kelon locally we recommend you to use Docker to build a container.
```bash
# Clone the repository
$ git clone git@github.com:Foundato/kelon.git

# Build the container
$ docker build . -t kelon:latest

# Or install kelon on your local machine
$ go mod download
$ go build -o ./kelon ./cmd/kelon
```

After kelon is installed, you have to create following configuration files that tell Kelon which data sources it should connect to and how it should map incoming requests to OPA-Queries.

### Configuration

#### datastore.yml

Defines all available datastores and their entities

```yaml
# Datastores to connect to
datastores:
  mysql:                              # Datastore's alias (it will be used throughout all your policies)
    type: mysql                       # Datastore type
    connection:                       # Information about the datastore connection
      host: localhost
      port: 3306
      database: appstore
      user: You
      password: SuperSecure

# Entity-Schemas define the entities of one schema inside a datastore
entity_schemas:
  mysql:                                # Datastore alias
    appstore:                           # Target schema
      entities:                         # List of all entities of the schema
        - users
        - app_rights
        - apps
```

#### call-operands/mysql.yml

Open Policy Agent has [builtin call operands](https://www.openpolicyagent.org/docs/latest/policy-language/#operators) which may not be the same as the ones of each datasource.
Therefore each datasource-type has to have a mappings-file inside the folder 'call-operands'.

```yaml
# Call operands map OPA's functions to datastore-native ones.
# You find more advanced files in directory /call-operands
call-operands:
  # Relational operands
  - op: eq
    args: 2
    mapping: "$0 = $1"
  - op: equal
    args: 2
    mapping: "$0 = $1"
```

#### api.yml

Map incoming api-requests to OPA-Queries executed on data sources.

```yaml
# Mappings for the APIs that your services expose
apis:
  # Route all requests starting with /api to datastore with alias 'mysql'
  - path-prefix: /api
    datastore: mysql
    # Mappings for incoming paths (the most specific mapping is picked in case of multiple mappings)
    mappings:
      - path: /apps/.*                   # Matches [GET, POST] /api/apps/.*
        package: applications            # Maps to OPA-Query 'data.applications.allow == true'
        # If no methods are supplied, all methods are matched
        methods:
          - GET
          - POST
```

#### opa.yml

Internally used by Open Policy Agent.

```yaml
labels:
  app: Kelon
  region: europe-west3
  environment: development

decision_logs:
  console: true
  reporting:
    min_delay_seconds: 300
    max_delay_seconds: 600

default_decision: /http/authz/allow
```

#### policies/applications.rego

Contains OPA-Regos. Note the unknown 'mysql' here which is automatically used by Kelon to translate the 

request to path: '/api/apps/3' 

into the SQL-Statement: 'SELECT count(*) FROM appstore.apps WHERE apps.stars = 5 and apps.id = 3'

```rego
package applications

# Deny all by default
allow = false

# Path: GET /api/apps/<number>
# All apps withe 5 stars can be viewed
allow = true {
    input.method = "GET"
    input.path = ["api", "apps", appId]

    data.mysql.apps[app].stars == 5
    app.id == appId
}
```

### Run Kelon

To run Kelon, there has to be a available MySQL database with the specified connection.
Feel free to use the MySQL-Database from our example (which is already configured for above configs).

```bash
$ docker-compose up -d mysql
```

Now you should be able to run kelon:

```bash
$ kelon start
```
