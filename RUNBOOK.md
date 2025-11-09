# Runbook

This documents some maintenance tasks for esbuild so I don't forget how they
work. There are a lot of moving parts now that esbuild uses trusted publishing.

## Publishing a release

Publishing a release is now done by using GitHub Actions as a
[trusted publisher](https://docs.npmjs.com/trusted-publishers).
To publish a new release:

1. Update the version in [`version.txt`](./version.txt). Only include the number. Do not include a leading `v`.
2. Copy that version verbatim (without the leading `v`) to a `##` header in [`CHANGELOG.md`](./CHANGELOG.md). This usually replaces the `## Unreleased` header used for unreleased changes.
3. Run `make platform-all` to update the version number in all `package.json` files. The publishing workflow will fail without this step.
4. Commit and push using a message such as `publish 0.X.Y to npm`. This should trigger the publishing workflow described below.

Pushing a change to [`version.txt`](./version.txt) causes the following:

- The [`publish.yml`](./.github/workflows/publish.yml) workflow in this repo
  will be triggered, which will:

    1. Build and publish all npm packages to npm using trusted publishing
    2. Create a tag for the release that looks like `v0.X.Y`
    3. Publish a [GitHub Release](https://github.com/evanw/esbuild/releases) containing the release notes in [`CHANGELOG.md`](./CHANGELOG.md)

- The [`release.yml`](https://github.com/esbuild/deno-esbuild/blob/main/.github/workflows/release.yml)
  workflow in the https://github.com/esbuild/deno-esbuild repo runs
  occasionally. On the next run, it will notice the version change and:

    1. Clone this repo
    2. Run `make platform-deno`
    3. Commit and push the new contents of the `deno` folder to the `deno-esbuild` repo
    4. Create a tag for the release that looks like `v0.X.Y`
    5. Post an event to the https://api.deno.land/webhook/gh/esbuild webhook
    6. Deno will then add a new version to https://deno.land/x/esbuild

    You can also manually trigger this workflow if you want it to happen immediately.

- The [`release.yml`](https://github.com/esbuild/esbuild.github.io/blob/main/.github/workflows/release.yml)
  workflow in the https://github.com/esbuild/esbuild.github.io repo runs
  occasionally. On the next run, it will notice the version change and:

    1. Create a new `dl/v0.X.Y` script for the new version number
    2. Update the `dl/latest` script with the new version number
    3. Commit and push these new scripts to the `gh-pages` branch of the `esbuild.github.io` repo
    4. GitHub Pages will then deploy these updates to https://esbuild.github.io/

    You can also manually trigger this workflow if you want it to happen immediately.

## Adding a new package

Each platform (operating system + architecture) needs a separate optional npm
package due to how esbuild's installer works. New packages should be created
under the `@esbuild/` scope so it's obvious that they are official.

Create a directory for the new package inside the [`npm/@esbuild`](./npm/@esbuild/)
directory. Then modify the rest of the repo to reference the new package. The
specifics for what to modify depends on the platform, but a good place to
start is to search for the name of a similar existing package and see where
it's used.

In addition, you'll need to prepare that package for the next release. To do
that:

1. Create an empty package with the expected name and a version of 0.0.1
2. Publish it with `npm publish --access public` (note that scoped packages are private by default)
3. Log in to the npm website and go to the package settings
4. Ensure that the only maintainer is the [esbuild](https://www.npmjs.com/~esbuild) user
5. Add the GitHub repo as the trusted publisher:
    - **Organization or user:** `evanw`
    - **Repository:** `esbuild`
    - **Workflow filename:** `publish.yml`
6. Ensure publishing access is set to **Require two-factor authentication and disallow tokens (recommended)**
