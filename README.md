# Terraform Provider -  Bitwarden Secrets Manager

_This Terraform provider is built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)._

The purpose of this Terraform Provider is to streamline the process of using Bitwarden Secrets Manager within Terraform and OpenTofu, making it more secure and efficient.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.5
- [Go](https://golang.org/doc/install) >= 1.23.0

## Developing the provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

### Building the provider

#### Local development

1. Clone the repository
2. Enter the repository directory
3. Execute `go mod tidy` to install all dependencies
4. Build the provider using the Go `install` command:
    ```shell
    go install .
    ```

This will build a dynamically linked binary for the provider and puts it in the `$GOPATH/bin` directory.

#### CGO and statically linked binaries

This provider is using the official [`sdk-go`](https://github.com/bitwarden/sdk-go) from [Bitwarden](https://github.com/bitwarden).
This dependency utilizes `CGO`.
In order to build a statically linked binary for linux, the following build configuration is necessary:

1. The library `musl-tools` needs to be available on the system
2. The following environment variables need to be set:
    ```bash
    go env -w CGO_ENABLED="1"
    go env -w CC="musl-gcc"
    go env -w CGO_LDFLAGS="-static -Wl,-unresolved-symbols=ignore-all"
    ```
   
Using this configuration, `go install .` and `go build` should generate statically linked binaries.

### Development overrides

In order to tell `terraform` to use the local build of the provider, add a `dev_override`.
Therefore, create or open the file `~/.terraformrc` and add an entry for the `bitwarden-secrets` provider:

```text
provider_installation {
  dev_overrides {
      "registry.terraform.io/bitwarden/bitwarden-secrets" = "/Users/user-name/go/bin"
      "registry.opentofu.org/bitwarden/bitwarden-secrets" = "/Users/user-name/go/bin"
  }

  # For all other providers, install them directly from their origin provider
  # registries as normal. If you omit this, Terraform will _only_ use
  # the dev_overrides block, and so no other providers will be available.
  direct {}
}
```

### Creating documentation

The usage documentation of the provider can be found inside the [`/docs`](./docs) folder.
This documentation is partly generated automatically from the source code and partly written by hand.
It uses the [`tfplugindocs`](https://github.com/hashicorp/terraform-plugin-docs) and Hashicorp's official guides on how to write good [provider documentation](https://developer.hashicorp.com/terraform/registry/providers/docs).

To generate or update documentation, run `go generate`.

```
// Run "go generate" to format example terraform files and generate the docs for the registry/website

// If you do not have terraform installed, you can remove the formatting command, but it is suggested to
// ensure the documentation is formatted properly.
//go:generate terraform fmt -recursive ./examples/

// Run the docs generation tool, check its repository for more information on how it works and how docs
// can be customized.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate
```

### Adding dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up-to-date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

### Acceptance Tests

In order to run the full suite of Acceptance tests, you need to provide the following 2 `.env` files:

1. `.env.local.test`
2. `.env.local.no.access`

Both files should contain the following configuration values:

```text
BW_API_URL="https://your-api-test-endpoint.example.com"
BW_IDENTITY_API_URL="https://your-identity-test-endpoint.example.com"
BW_ACCESS_TOKEN="<your machine account access token >"
BW_ORGANIZATION_ID="< organization id  >"
BW_STATE_FILE=".bw-state-test"
```

*Important:* The second file `.env.local.no.access` needs to be configured with an access token belonging to a machine account with no project access.
The file [`test_utils.go`](./internal/provider/test_utils.go) uses this file to create the necessary provider configuration.

*Note:* Acceptance tests create real resources, and often cost money to run.

If everything is provided, one can execute all acceptance tests with `make`:

#### Testing with `terraform` CLI

```shell
make testacc
```

#### Testing with `tofu` CLI

In order to run acceptance tests using the [OpenTofu](https://opentofu.org/) engine instead of Terraform, one needs to install the CLI first:
https://opentofu.org/.

Thereafter, the [`GNUmakefile`](./GNUmakefile) contains a specific command to run the acceptance test suite using the `tofu` CLI:

```shell
make testacc_tofu
```

### SDK request serialization

SDK network operations share one capacity-one gate per configured provider instance,
including access-token login, secret CRUD, secret reads/listing, and project listing.
Only the SDK call holds the gate; password generation and Terraform state and
diagnostics remain outside it. Other providers can still run concurrently.

This is an initial mitigation for concurrent-request 429 errors, not a rate-limit
guarantee. Sequential requests can still exceed a requests-per-second quota, and
separate provider configurations, processes, and CI runs are not coordinated.
Cancellation stops waiting for the gate and is checked again before invoking the
SDK. It cannot interrupt a native SDK call that has already started.

The implementation is based on upstream `v1.0.1`
(`f1f791ee0bf63d6a27f16c00f00bc718973026cc`), with the unchanged
`github.com/bitwarden/sdk-go/v2 v2.1.0` dependency. That SDK delegates operations
through its native command runner, exposes API failures as plain error strings,
and provides neither an injectable Go HTTP client nor structured status or
`Retry-After` metadata through these interfaces.

There are no retries or delays. If 429s persist, the next increment is bounded
429 retries with a shared cooldown inside the same gate; structured status/header
support may require SDK work.

Run the offline serialization tests (including race detection) with:

```shell
go test -race ./... -run '^TestRequestGate' -timeout=60s
```

Do not use the full acceptance suite for an offline check: its setup creates live
resources. Build locally as described above and point a development override at
the resulting binary directory. In an isolated scratch Terraform directory, use
that override to run `terraform providers schema -json` to check plugin startup
without authenticating or creating secrets. Adoption in infrastructure is separate.

### Testing the Systrade build alongside the official provider

The `systrade/local-build` branch identifies this build as `1.0.2-systrade.1`.
Its version and these instructions are separate from the serialization commit,
so they can be omitted from an upstream contribution. The provider source address
remains `bitwarden/bitwarden-secrets`.

Build from this branch into a dedicated directory (Go and a C compiler required):

```sh
GOTOOLCHAIN=go1.26.4 go build \
  -o /tmp/bitwarden-systrade/bin/terraform-provider-bitwarden-secrets .
cat > /tmp/bitwarden-systrade/terraform.tfrc <<'CONFIG'
provider_installation {
  dev_overrides {
    "registry.terraform.io/bitwarden/bitwarden-secrets" = "/tmp/bitwarden-systrade/bin"
  }
  direct {}
}
CONFIG
```

Select that override for individual commands in a separate test configuration:

```sh
TF_CLI_CONFIG_FILE=/tmp/bitwarden-systrade/terraform.tfrc \
  terraform providers schema -json
```

The test configuration must declare `bitwarden/bitwarden-secrets` in
`required_providers`. Schema inspection does not authenticate or create secrets.
Commands without this CLI configuration continue using their normal provider
installation settings. Avoid a global override or exporting `TF_CLI_CONFIG_FILE`
when comparing builds. Use separate test state and resources for any later live tests.

A development override selects the local binary regardless of the configured
version constraint or locked provider checksum; `terraform init` still performs
normal version selection. Changing `ProviderVersion` alone does not install the
fork, and provider aliases cannot select two versions of the same source in one
configuration. For a future registry or mirror installation of this prerelease,
use an exact constraint: `version = "= 1.0.2-systrade.1"`.

The inherited release workflow still uses Bitwarden's signing credentials and
artifacts from `main`; it is not a Systrade publishing setup. Local testing does
not require running it or publishing a release.
