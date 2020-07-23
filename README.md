# Kelon

Kelon is a policy enforcement point, that is wrapping the [Open Policy Agent](https://www.openpolicyagent.org) (OPA) and adding more functionality in terms of microservices.

### Status
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=alert_status)](https://sonarcloud.io/dashboard?id=Foundato_kelon)
[![Lines of Code](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=ncloc)](https://sonarcloud.io/dashboard?id=Foundato_kelon)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FFoundato%2Fkelon.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FFoundato%2Fkelon?ref=badge_shield)

[![Code Smells](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=code_smells)](https://sonarcloud.io/dashboard?id=Foundato_kelon)
[![Bugs](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=bugs)](https://sonarcloud.io/dashboard?id=Foundato_kelon)
[![Duplicated Lines (%)](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=duplicated_lines_density)](https://sonarcloud.io/dashboard?id=Foundato_kelon)

[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=sqale_rating)](https://sonarcloud.io/dashboard?id=Foundato_kelon)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=security_rating)](https://sonarcloud.io/dashboard?id=Foundato_kelon)
[![Vulnerabilities](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=vulnerabilities)](https://sonarcloud.io/dashboard?id=Foundato_kelon)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=Foundato_kelon&metric=reliability_rating)](https://sonarcloud.io/dashboard?id=Foundato_kelon)


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

To show you the capabilities of Kelon in action, we provided a simple [example setup of Kelon](./examples/docker-compose) with three databases [My-SQL, PostgreSQL, Mongo-DB].
In order to run this example you need to install [Docker](https://docs.docker.com/install/) and [Docker-Compose](https://docs.docker.com/compose/install/) and [Postman (optional)](https://www.postman.com/downloads/).
Afterwards you can run the example like this:

```bash
$ git clone git@github.com:Foundato/kelon.git
$ cd kelon
$ docker-compose up -d
```

After everything is up and running, you can use this [Postman-Collection](./examples/kelon_example_E2E.postman_collection.json) to verify that kelon is working correctly.


# Want to know more about Kelon?

Then visit our [official docs](https://docs.kelon.io/).

## License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FFoundato%2Fkelon.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2FFoundato%2Fkelon?ref=badge_large)