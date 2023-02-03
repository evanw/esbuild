ESBUILD_VERSION = $(shell cat version.txt)

# Strip debug info
GO_FLAGS += "-ldflags=-s -w"

# Avoid embedding the build path in the executable for more reproducible builds
GO_FLAGS += -trimpath

esbuild: version-go cmd/esbuild/*.go pkg/*/*.go internal/*/*.go go.mod
	CGO_ENABLED=0 go build $(GO_FLAGS) ./cmd/esbuild

test:
	@$(MAKE) --no-print-directory -j6 test-common

# These tests are for development
test-common: test-go vet-go no-filepath verify-source-map end-to-end-tests js-api-tests plugin-tests register-test node-unref-tests

# These tests are for release (the extra tests are not included in "test" because they are pretty slow)
test-all:
	@$(MAKE) --no-print-directory -j6 test-common test-deno ts-type-tests test-wasm-node test-wasm-browser lib-typecheck test-yarnpnp

check-go-version:
	@go version | grep ' go1\.20 ' || (echo 'Please install Go version 1.20.0' && false)

# Note: Don't add "-race" here by default. The Go race detector is currently
# only supported on the following configurations:
#
#   darwin/amd64
#   darwin/arm64
#   freebsd/amd64,
#   linux/amd64
#   linux/arm64
#   linux/ppc64le
#   netbsd/amd64
#   windows/amd64
#
# Also, it isn't necessarily supported on older OS versions even if the OS/CPU
# combination is supported, such as on macOS 10.9. If you want to test using
# the race detector, you can manually add it using the ESBUILD_RACE environment
# variable like this: "ESBUILD_RACE=-race make test". Or you can permanently
# enable it by adding "export ESBUILD_RACE=-race" to your shell profile.
test-go:
	go test $(ESBUILD_RACE) ./internal/...

vet-go:
	go vet ./cmd/... ./internal/... ./pkg/...

fmt-go:
	test -z "$(shell go fmt ./cmd/... ./internal/... ./pkg/... )"

no-filepath:
	@! grep --color --include '*.go' -r '"path/filepath"' cmd internal pkg || ( \
		echo 'error: Use of "path/filepath" is disallowed. See http://golang.org/issue/43768.' && false)

# This uses "env -i" to run in a clean environment with no environment
# variables. It then adds some environment variables back as needed.
# This is a hack to avoid a problem with the WebAssembly support in Go
# 1.17.2, which will crash when run in an environment with over 4096
# bytes of environment variable data such as GitHub Actions.
test-wasm-node: esbuild
	env -i $(shell go env) PATH="$(shell go env GOROOT)/misc/wasm:$(PATH)" GOOS=js GOARCH=wasm go test ./internal/...
	node scripts/wasm-tests.js

test-wasm-browser: platform-wasm | scripts/browser/node_modules
	cd scripts/browser && node browser-tests.js

test-deno: esbuild platform-deno
	ESBUILD_BINARY_PATH="$(shell pwd)/esbuild" deno test --allow-run --allow-env --allow-net --allow-read --allow-write --no-check scripts/deno-tests.js
	@echo '✅ deno tests passed' # I couldn't find a Deno API for telling when tests have failed, so I'm doing this here instead
	deno eval 'import { transform, stop } from "file://$(shell pwd)/deno/mod.js"; console.log((await transform("1+2")).code); stop()' | grep "1 + 2;"
	deno eval 'import { transform, stop } from "file://$(shell pwd)/deno/wasm.js"; console.log((await transform("1+2")).code); stop()' | grep "1 + 2;"

test-deno-windows: esbuild platform-deno
	ESBUILD_BINARY_PATH=./esbuild.exe deno test --allow-run --allow-env --allow-net --allow-read --allow-write --no-check scripts/deno-tests.js

register-test: version-go | scripts/node_modules
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/register-test.js

verify-source-map: version-go | scripts/node_modules
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/verify-source-map.js

end-to-end-tests: version-go
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/end-to-end-tests.js

js-api-tests: version-go
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/js-api-tests.js

plugin-tests: version-go
	node scripts/plugin-tests.js

ts-type-tests: | scripts/node_modules
	node scripts/ts-type-tests.js

require/old-ts/node_modules:
	cd require/old-ts && npm ci

test-old-ts: platform-neutral | require/old-ts/node_modules
	rm -fr scripts/.test-old-ts && mkdir scripts/.test-old-ts
	cp `find npm/esbuild -name '*.d.ts'` scripts/.test-old-ts
	cd scripts/.test-old-ts && ../../require/old-ts/node_modules/.bin/tsc *.d.ts
	rm -fr scripts/.test-old-ts

node-unref-tests: | scripts/node_modules
	node scripts/node-unref-tests.js

lib-typecheck: | lib/node_modules
	cd lib && node_modules/.bin/tsc -noEmit -p tsconfig.json
	cd lib && node_modules/.bin/tsc -noEmit -p tsconfig-deno.json

# End-to-end tests
test-e2e: test-e2e-npm test-e2e-pnpm test-e2e-yarn-berry test-e2e-deno

test-e2e-npm:
	# Test normal install
	rm -fr e2e-npm && mkdir e2e-npm && cd e2e-npm && echo {} > package.json && npm i esbuild
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test CI reinstall
	cd e2e-npm && npm ci
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test rebuild
	cd e2e-npm && npm rebuild && npm rebuild
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"

	# Test install without scripts
	rm -fr e2e-npm && mkdir e2e-npm && cd e2e-npm && echo {} > package.json && npm i --ignore-scripts esbuild
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test CI reinstall
	cd e2e-npm && npm ci
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test rebuild
	cd e2e-npm && npm rebuild && npm rebuild
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"

	# Test install without optional dependencies
	rm -fr e2e-npm && mkdir e2e-npm && cd e2e-npm && echo {} > package.json && npm i --no-optional esbuild
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test CI reinstall
	cd e2e-npm && npm ci
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test rebuild
	cd e2e-npm && npm rebuild && npm rebuild
	cd e2e-npm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"

	# Clean up
	rm -fr e2e-npm

test-e2e-pnpm:
	# Test normal install
	rm -fr e2e-pnpm && mkdir e2e-pnpm && cd e2e-pnpm && echo {} > package.json && pnpm i esbuild
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test CI reinstall
	cd e2e-pnpm && pnpm i --frozen-lockfile
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test rebuild
	cd e2e-pnpm && pnpm rebuild && pnpm rebuild
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"

	# Test install without scripts
	rm -fr e2e-pnpm && mkdir e2e-pnpm && cd e2e-pnpm && echo {} > package.json && pnpm i --ignore-scripts esbuild
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test CI reinstall
	cd e2e-pnpm && pnpm i --frozen-lockfile
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test rebuild
	cd e2e-pnpm && pnpm rebuild && pnpm rebuild
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"

	# Test install without optional dependencies
	rm -fr e2e-pnpm && mkdir e2e-pnpm && cd e2e-pnpm && echo {} > package.json && pnpm i --no-optional esbuild
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test CI reinstall
	cd e2e-pnpm && pnpm i --frozen-lockfile
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"
	# Test rebuild
	cd e2e-pnpm && pnpm rebuild && pnpm rebuild
	cd e2e-pnpm && echo "1+2" | node_modules/.bin/esbuild | grep "1 + 2;" && node -p "require('esbuild').transformSync('1+2').code" | grep "1 + 2;"

	# Clean up
	rm -fr e2e-pnpm

test-e2e-yarn-berry:
	# Test normal install
	rm -fr e2e-yb && mkdir e2e-yb && cd e2e-yb && echo {} > package.json && touch yarn.lock && yarn set version berry && yarn add esbuild
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"
	# Test CI reinstall
	cd e2e-yb && yarn install --immutable
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"
	# Test rebuild
	cd e2e-yb && yarn rebuild && yarn rebuild
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"

	# Test install without scripts
	rm -fr e2e-yb && mkdir e2e-yb && cd e2e-yb && echo {} > package.json && touch yarn.lock && echo 'enableScripts: false' > .yarnrc.yml && yarn set version berry && yarn add esbuild
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"
	# Test CI reinstall
	cd e2e-yb && yarn install --immutable
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"
	# Test rebuild
	cd e2e-yb && yarn rebuild && yarn rebuild
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"

	# Test install without optional dependencies
	rm -fr e2e-yb && mkdir e2e-yb && cd e2e-yb && echo {} > package.json && touch yarn.lock && yarn set version berry && yarn add --no-optional esbuild
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"
	# Test CI reinstall
	cd e2e-yb && yarn install --immutable
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"
	# Test rebuild
	cd e2e-yb && yarn rebuild && yarn rebuild
	cd e2e-yb && echo "1+2" | yarn esbuild && yarn node -p "require('esbuild').transformSync('1+2').code"

	# Clean up
	rm -fr e2e-yb

test-e2e-deno:
	deno eval 'import { transform, stop } from "https://deno.land/x/esbuild@v$(ESBUILD_VERSION)/mod.js"; console.log((await transform("1+2")).code); stop()' | grep "1 + 2;"
	deno eval 'import { transform, stop } from "https://deno.land/x/esbuild@v$(ESBUILD_VERSION)/wasm.js"; console.log((await transform("1+2")).code); stop()' | grep "1 + 2;"

test-yarnpnp: platform-wasm
	node scripts/test-yarnpnp.js

# Note: This used to only be rebuilt when "version.txt" was newer than
# "cmd/esbuild/version.go", but that caused the publishing script to publish
# invalid builds in the case when the publishing script failed once, the change
# to "cmd/esbuild/version.go" was reverted, and then the publishing script was
# run again, since in that case "cmd/esbuild/version.go" has a later mtime than
# "version.txt" but is still outdated.
#
# To avoid this problem, we now always run this step regardless of mtime status.
# This step still avoids writing to "cmd/esbuild/version.go" if it already has
# the correct contents, so it won't unnecessarily invalidate anything that uses
# "cmd/esbuild/version.go" as a dependency.
version-go:
	node scripts/esbuild.js --update-version-go

platform-all:
	@$(MAKE) --no-print-directory -j4 \
		platform-android-arm \
		platform-android-arm64 \
		platform-android-x64 \
		platform-darwin-arm64 \
		platform-darwin-x64 \
		platform-deno \
		platform-freebsd-arm64 \
		platform-freebsd-x64 \
		platform-linux-arm \
		platform-linux-arm64 \
		platform-linux-ia32 \
		platform-linux-loong64 \
		platform-linux-mips64el \
		platform-linux-ppc64 \
		platform-linux-riscv64 \
		platform-linux-s390x \
		platform-linux-x64 \
		platform-netbsd-x64 \
		platform-neutral \
		platform-openbsd-x64 \
		platform-sunos-x64 \
		platform-wasm \
		platform-win32-arm64 \
		platform-win32-ia32 \
		platform-win32-x64

platform-win32-x64: version-go
	node scripts/esbuild.js npm/@esbuild/win32-x64/package.json --version
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GO_FLAGS) -o npm/@esbuild/win32-x64/esbuild.exe ./cmd/esbuild

platform-win32-ia32: version-go
	node scripts/esbuild.js npm/@esbuild/win32-ia32/package.json --version
	CGO_ENABLED=0 GOOS=windows GOARCH=386 go build $(GO_FLAGS) -o npm/@esbuild/win32-ia32/esbuild.exe ./cmd/esbuild

platform-win32-arm64: version-go
	node scripts/esbuild.js npm/@esbuild/win32-arm64/package.json --version
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(GO_FLAGS) -o npm/@esbuild/win32-arm64/esbuild.exe ./cmd/esbuild

platform-unixlike: version-go
	@test -n "$(GOOS)" || (echo "The environment variable GOOS must be provided" && false)
	@test -n "$(GOARCH)" || (echo "The environment variable GOARCH must be provided" && false)
	@test -n "$(NPMDIR)" || (echo "The environment variable NPMDIR must be provided" && false)
	node scripts/esbuild.js "$(NPMDIR)/package.json" --version
	CGO_ENABLED=0 GOOS="$(GOOS)" GOARCH="$(GOARCH)" go build $(GO_FLAGS) -o "$(NPMDIR)/bin/esbuild" ./cmd/esbuild

platform-android-x64: platform-wasm
	node scripts/esbuild.js npm/@esbuild/android-x64/package.json --version

platform-android-arm: platform-wasm
	node scripts/esbuild.js npm/@esbuild/android-arm/package.json --version

platform-android-arm64:
	@$(MAKE) --no-print-directory GOOS=android GOARCH=arm64 NPMDIR=npm/@esbuild/android-arm64 platform-unixlike

platform-darwin-x64:
	@$(MAKE) --no-print-directory GOOS=darwin GOARCH=amd64 NPMDIR=npm/@esbuild/darwin-x64 platform-unixlike

platform-darwin-arm64:
	@$(MAKE) --no-print-directory GOOS=darwin GOARCH=arm64 NPMDIR=npm/@esbuild/darwin-arm64 platform-unixlike

platform-freebsd-x64:
	@$(MAKE) --no-print-directory GOOS=freebsd GOARCH=amd64 NPMDIR=npm/@esbuild/freebsd-x64 platform-unixlike

platform-freebsd-arm64:
	@$(MAKE) --no-print-directory GOOS=freebsd GOARCH=arm64 NPMDIR=npm/@esbuild/freebsd-arm64 platform-unixlike

platform-netbsd-x64:
	@$(MAKE) --no-print-directory GOOS=netbsd GOARCH=amd64 NPMDIR=npm/@esbuild/netbsd-x64 platform-unixlike

platform-openbsd-x64:
	@$(MAKE) --no-print-directory GOOS=openbsd GOARCH=amd64 NPMDIR=npm/@esbuild/openbsd-x64 platform-unixlike

platform-linux-x64:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=amd64 NPMDIR=npm/@esbuild/linux-x64 platform-unixlike

platform-linux-ia32:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=386 NPMDIR=npm/@esbuild/linux-ia32 platform-unixlike

platform-linux-arm:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=arm NPMDIR=npm/@esbuild/linux-arm platform-unixlike

platform-linux-arm64:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=arm64 NPMDIR=npm/@esbuild/linux-arm64 platform-unixlike

platform-linux-loong64:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=loong64 NPMDIR=npm/@esbuild/linux-loong64 platform-unixlike

platform-linux-mips64el:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=mips64le NPMDIR=npm/@esbuild/linux-mips64el platform-unixlike

platform-linux-ppc64:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=ppc64le NPMDIR=npm/@esbuild/linux-ppc64 platform-unixlike

platform-linux-riscv64:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=riscv64 NPMDIR=npm/@esbuild/linux-riscv64 platform-unixlike

platform-linux-s390x:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=s390x NPMDIR=npm/@esbuild/linux-s390x platform-unixlike

platform-sunos-x64:
	@$(MAKE) --no-print-directory GOOS=illumos GOARCH=amd64 NPMDIR=npm/@esbuild/sunos-x64 platform-unixlike

platform-wasm: esbuild
	node scripts/esbuild.js npm/esbuild-wasm/package.json --version
	node scripts/esbuild.js ./esbuild --wasm

platform-neutral: esbuild
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/esbuild.js ./esbuild --neutral

platform-deno: platform-wasm
	node scripts/esbuild.js ./esbuild --deno

publish-all: check-go-version
	@npm --version > /dev/null || (echo "The 'npm' command must be in your path to publish" && false)
	@echo "Checking for uncommitted/untracked changes..." && test -z "`git status --porcelain | grep -vE 'M (CHANGELOG\.md|version\.txt)'`" || \
		(echo "Refusing to publish with these uncommitted/untracked changes:" && \
		git status --porcelain | grep -vE 'M (CHANGELOG\.md|version\.txt)' && false)
	@echo "Checking for main branch..." && test main = "`git rev-parse --abbrev-ref HEAD`" || \
		(echo "Refusing to publish from non-main branch `git rev-parse --abbrev-ref HEAD`" && false)
	@echo "Checking for unpushed commits..." && git fetch
	@test "" = "`git cherry`" || (echo "Refusing to publish with unpushed commits" && false)

	# Prebuild now to prime go's compile cache and avoid timing issues later
	@$(MAKE) --no-print-directory platform-all

	# Commit now before publishing so git is clean for this: https://github.com/golang/go/issues/37475
	# Note: If this fails, then the version number was likely not incremented before running this command
	git commit -am "publish $(ESBUILD_VERSION) to npm"
	git tag "v$(ESBUILD_VERSION)"
	@test -z "`git status --porcelain`" || (echo "Aborting because git is somehow unclean after a commit" && false)

	# Make sure the npm directory is pristine (including .gitignored files) since it will be published
	rm -fr npm && git checkout npm

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-win32-x64 \
		publish-win32-ia32 \
		publish-win32-arm64 \
		publish-sunos-x64

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-freebsd-x64 \
		publish-freebsd-arm64 \
		publish-openbsd-x64 \
		publish-netbsd-x64

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-android-x64 \
		publish-android-arm \
		publish-android-arm64 \
		publish-darwin-x64

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-darwin-arm64 \
		publish-linux-x64 \
		publish-linux-ia32 \
		publish-linux-arm

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-linux-arm64 \
		publish-linux-riscv64 \
		publish-linux-loong64 \
		publish-linux-mips64el

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-linux-ppc64 \
		publish-linux-s390x

	# Do these last to avoid race conditions
	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-neutral \
		publish-deno \
		publish-wasm \
		publish-dl

	git push origin main "v$(ESBUILD_VERSION)"

publish-win32-x64: platform-win32-x64
	test -n "$(OTP)" && cd npm/@esbuild/win32-x64 && npm publish --otp="$(OTP)"

publish-win32-ia32: platform-win32-ia32
	test -n "$(OTP)" && cd npm/@esbuild/win32-ia32 && npm publish --otp="$(OTP)"

publish-win32-arm64: platform-win32-arm64
	test -n "$(OTP)" && cd npm/@esbuild/win32-arm64 && npm publish --otp="$(OTP)"

publish-android-x64: platform-android-x64
	test -n "$(OTP)" && cd npm/@esbuild/android-x64 && npm publish --otp="$(OTP)"

publish-android-arm: platform-android-arm
	test -n "$(OTP)" && cd npm/@esbuild/android-arm && npm publish --otp="$(OTP)"

publish-android-arm64: platform-android-arm64
	test -n "$(OTP)" && cd npm/@esbuild/android-arm64 && npm publish --otp="$(OTP)"

publish-darwin-x64: platform-darwin-x64
	test -n "$(OTP)" && cd npm/@esbuild/darwin-x64 && npm publish --otp="$(OTP)"

publish-darwin-arm64: platform-darwin-arm64
	test -n "$(OTP)" && cd npm/@esbuild/darwin-arm64 && npm publish --otp="$(OTP)"

publish-freebsd-x64: platform-freebsd-x64
	test -n "$(OTP)" && cd npm/@esbuild/freebsd-x64 && npm publish --otp="$(OTP)"

publish-freebsd-arm64: platform-freebsd-arm64
	test -n "$(OTP)" && cd npm/@esbuild/freebsd-arm64 && npm publish --otp="$(OTP)"

publish-netbsd-x64: platform-netbsd-x64
	test -n "$(OTP)" && cd npm/@esbuild/netbsd-x64 && npm publish --otp="$(OTP)"

publish-openbsd-x64: platform-openbsd-x64
	test -n "$(OTP)" && cd npm/@esbuild/openbsd-x64 && npm publish --otp="$(OTP)"

publish-linux-x64: platform-linux-x64
	test -n "$(OTP)" && cd npm/@esbuild/linux-x64 && npm publish --otp="$(OTP)"

publish-linux-ia32: platform-linux-ia32
	test -n "$(OTP)" && cd npm/@esbuild/linux-ia32 && npm publish --otp="$(OTP)"

publish-linux-arm: platform-linux-arm
	test -n "$(OTP)" && cd npm/@esbuild/linux-arm && npm publish --otp="$(OTP)"

publish-linux-arm64: platform-linux-arm64
	test -n "$(OTP)" && cd npm/@esbuild/linux-arm64 && npm publish --otp="$(OTP)"

publish-linux-loong64: platform-linux-loong64
	test -n "$(OTP)" && cd npm/@esbuild/linux-loong64 && npm publish --otp="$(OTP)"

publish-linux-mips64el: platform-linux-mips64el
	test -n "$(OTP)" && cd npm/@esbuild/linux-mips64el && npm publish --otp="$(OTP)"

publish-linux-ppc64: platform-linux-ppc64
	test -n "$(OTP)" && cd npm/@esbuild/linux-ppc64 && npm publish --otp="$(OTP)"

publish-linux-riscv64: platform-linux-riscv64
	test -n "$(OTP)" && cd npm/@esbuild/linux-riscv64 && npm publish --otp="$(OTP)"

publish-linux-s390x: platform-linux-s390x
	test -n "$(OTP)" && cd npm/@esbuild/linux-s390x && npm publish --otp="$(OTP)"

publish-sunos-x64: platform-sunos-x64
	test -n "$(OTP)" && cd npm/@esbuild/sunos-x64 && npm publish --otp="$(OTP)"

publish-wasm: platform-wasm
	test -n "$(OTP)" && cd npm/esbuild-wasm && npm publish --otp="$(OTP)"

publish-neutral: platform-neutral
	test -n "$(OTP)" && cd npm/esbuild && npm publish --otp="$(OTP)"

publish-deno:
	test -d deno/.git || (rm -fr deno && git clone git@github.com:esbuild/deno-esbuild.git deno)
	cd deno && git fetch && git checkout main && git reset --hard origin/main
	@$(MAKE) --no-print-directory platform-deno
	cd deno && git add mod.js mod.d.ts wasm.js wasm.d.ts esbuild.wasm
	cd deno && git commit -m "publish $(ESBUILD_VERSION) to deno"
	cd deno && git tag "v$(ESBUILD_VERSION)"
	cd deno && git push origin main "v$(ESBUILD_VERSION)"

publish-dl:
	test -d www/.git || (rm -fr www && git clone git@github.com:esbuild/esbuild.github.io.git www)
	cd www && git fetch && git checkout gh-pages && git reset --hard origin/gh-pages
	cd www && cat ../dl.sh | sed 's/$$ESBUILD_VERSION/$(ESBUILD_VERSION)/' > dl/latest
	cd www && cat ../dl.sh | sed 's/$$ESBUILD_VERSION/$(ESBUILD_VERSION)/' > "dl/v$(ESBUILD_VERSION)"
	cd www && git add dl/latest "dl/v$(ESBUILD_VERSION)"
	cd www && git commit -m "publish download script for $(ESBUILD_VERSION)"
	cd www && git push origin gh-pages

validate-build:
	@test -n "$(TARGET)" || (echo "The environment variable TARGET must be provided" && false)
	@test -n "$(PACKAGE)" || (echo "The environment variable PACKAGE must be provided" && false)
	@test -n "$(SUBPATH)" || (echo "The environment variable SUBPATH must be provided" && false)
	@echo && echo "🔷 Checking $(SCOPE)$(PACKAGE)"
	@rm -fr validate && mkdir validate
	@$(MAKE) --no-print-directory "$(TARGET)"
	@curl -s "https://registry.npmjs.org/$(SCOPE)$(PACKAGE)/-/$(PACKAGE)-$(ESBUILD_VERSION).tgz" > validate/esbuild.tgz
	@cd validate && tar xf esbuild.tgz
	@ls -l "npm/$(SCOPE)$(PACKAGE)/$(SUBPATH)" "validate/package/$(SUBPATH)" && \
		shasum "npm/$(SCOPE)$(PACKAGE)/$(SUBPATH)" "validate/package/$(SUBPATH)" && \
		cmp "npm/$(SCOPE)$(PACKAGE)/$(SUBPATH)" "validate/package/$(SUBPATH)"
	@rm -fr validate

# This checks that the published binaries are bitwise-identical to the locally-build binaries
validate-builds:
	git fetch --all --tags && git checkout "v$(ESBUILD_VERSION)"
	@$(MAKE) --no-print-directory TARGET=platform-android-arm    SCOPE=@esbuild/ PACKAGE=android-arm     SUBPATH=esbuild.wasm validate-build
	@$(MAKE) --no-print-directory TARGET=platform-android-arm64  SCOPE=@esbuild/ PACKAGE=android-arm64   SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-android-x64    SCOPE=@esbuild/ PACKAGE=android-x64     SUBPATH=esbuild.wasm validate-build
	@$(MAKE) --no-print-directory TARGET=platform-darwin-arm64   SCOPE=@esbuild/ PACKAGE=darwin-arm64    SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-darwin-x64     SCOPE=@esbuild/ PACKAGE=darwin-x64      SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-freebsd-arm64  SCOPE=@esbuild/ PACKAGE=freebsd-arm64   SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-freebsd-x64    SCOPE=@esbuild/ PACKAGE=freebsd-x64     SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-arm      SCOPE=@esbuild/ PACKAGE=linux-arm       SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-arm64    SCOPE=@esbuild/ PACKAGE=linux-arm64     SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-ia32     SCOPE=@esbuild/ PACKAGE=linux-ia32      SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-loong64  SCOPE=@esbuild/ PACKAGE=linux-loong64   SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-mips64el SCOPE=@esbuild/ PACKAGE=linux-mips64el  SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-ppc64    SCOPE=@esbuild/ PACKAGE=linux-ppc64     SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-riscv64  SCOPE=@esbuild/ PACKAGE=linux-riscv64   SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-s390x    SCOPE=@esbuild/ PACKAGE=linux-s390x     SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-linux-x64      SCOPE=@esbuild/ PACKAGE=linux-x64       SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-netbsd-x64     SCOPE=@esbuild/ PACKAGE=netbsd-x64      SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-openbsd-x64    SCOPE=@esbuild/ PACKAGE=openbsd-x64     SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-sunos-x64      SCOPE=@esbuild/ PACKAGE=sunos-x64       SUBPATH=bin/esbuild  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-wasm                           PACKAGE=esbuild-wasm    SUBPATH=esbuild.wasm validate-build
	@$(MAKE) --no-print-directory TARGET=platform-win32-arm64    SCOPE=@esbuild/ PACKAGE=win32-arm64     SUBPATH=esbuild.exe  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-win32-ia32     SCOPE=@esbuild/ PACKAGE=win32-ia32      SUBPATH=esbuild.exe  validate-build
	@$(MAKE) --no-print-directory TARGET=platform-win32-x64      SCOPE=@esbuild/ PACKAGE=win32-x64       SUBPATH=esbuild.exe  validate-build

clean:
	go clean -cache
	go clean -testcache
	rm -f esbuild
	rm -f npm/@esbuild/win32-arm64/esbuild.exe
	rm -f npm/@esbuild/win32-ia32/esbuild.exe
	rm -f npm/@esbuild/win32-x64/esbuild.exe
	rm -f npm/esbuild-wasm/esbuild.wasm npm/esbuild-wasm/wasm_exec*.js
	rm -rf npm/@esbuild/android-arm/bin npm/@esbuild/android-arm/esbuild.wasm npm/@esbuild/android-arm/wasm_exec*.js
	rm -rf npm/@esbuild/android-arm64/bin
	rm -rf npm/@esbuild/android-x64/bin npm/@esbuild/android-x64/esbuild.wasm npm/@esbuild/android-x64/wasm_exec*.js
	rm -rf npm/@esbuild/darwin-arm64/bin
	rm -rf npm/@esbuild/darwin-x64/bin
	rm -rf npm/@esbuild/freebsd-arm64/bin
	rm -rf npm/@esbuild/freebsd-x64/bin
	rm -rf npm/@esbuild/linux-arm/bin
	rm -rf npm/@esbuild/linux-arm64/bin
	rm -rf npm/@esbuild/linux-ia32/bin
	rm -rf npm/@esbuild/linux-loong64/bin
	rm -rf npm/@esbuild/linux-mips64el/bin
	rm -rf npm/@esbuild/linux-ppc64/bin
	rm -rf npm/@esbuild/linux-riscv64/bin
	rm -rf npm/@esbuild/linux-s390x/bin
	rm -rf npm/@esbuild/linux-x64/bin
	rm -rf npm/@esbuild/netbsd-x64/bin
	rm -rf npm/@esbuild/openbsd-x64/bin
	rm -rf npm/@esbuild/sunos-x64/bin
	rm -rf npm/esbuild-wasm/esm
	rm -rf npm/esbuild-wasm/lib
	rm -rf npm/esbuild/bin npm/esbuild/lib npm/esbuild/install.js
	rm -rf require/*/bench/
	rm -rf require/*/demo/
	rm -rf require/*/node_modules/
	rm -rf require/yarnpnp/.pnp* require/yarnpnp/.yarn* require/yarnpnp/out*.js
	rm -rf validate

# This also cleans directories containing cached code from other projects
clean-all: clean
	rm -fr github demo bench

################################################################################
# These npm packages are used for benchmarks. Install them in subdirectories
# because we want to install the same package name at multiple versions

require/webpack5/node_modules:
	cd require/webpack5 && npm ci

require/rollup/node_modules:
	cd require/rollup && npm ci

require/parcel2/node_modules:
	cd require/parcel2 && npm ci

lib/node_modules:
	cd lib && npm ci

scripts/node_modules:
	cd scripts && npm ci

scripts/browser/node_modules:
	cd scripts/browser && npm ci

################################################################################
# This downloads the kangax compat-table and generates browser support mappings

github/compat-table:
	mkdir -p github/compat-table
	git clone --depth 1 https://github.com/kangax/compat-table.git github/compat-table

compat-table: | github/compat-table
	node scripts/compat-table.js

################################################################################
# This runs the test262 official JavaScript test suite through esbuild

github/test262:
	mkdir -p github
	git clone --depth 1 https://github.com/tc39/test262.git github/test262

demo/test262: | github/test262
	mkdir -p demo/test262
	cp -r github/test262/harness demo/test262/harness
	cp -r github/test262/test demo/test262/test

test262: esbuild | demo/test262
	node --experimental-vm-modules scripts/test262.js

################################################################################
# This runs UglifyJS's test suite through esbuild

github/uglify:
	mkdir -p github/uglify
	cd github/uglify && git init && git remote add origin https://github.com/mishoo/uglifyjs.git
	cd github/uglify && git fetch --depth 1 origin 860aa9531b2ce660ace8379c335bb092034b6e82 && git checkout FETCH_HEAD

demo/uglify: | github/uglify
	mkdir -p demo
	cp -r github/uglify/ demo/uglify
	cd demo/uglify && npm i

uglify: esbuild | demo/uglify
	node scripts/uglify-tests.js

################################################################################
# This builds Rollup using esbuild and then uses it to run Rollup's test suite

TEST_ROLLUP_FIND = "compilerOptions": {

TEST_ROLLUP_REPLACE += "compilerOptions": {
TEST_ROLLUP_REPLACE += "baseUrl": ".",
TEST_ROLLUP_REPLACE += "paths": { "package.json": [".\/package.json"] },

TEST_ROLLUP_FLAGS += --bundle
TEST_ROLLUP_FLAGS += --external:fsevents
TEST_ROLLUP_FLAGS += --outfile=dist/rollup.js
TEST_ROLLUP_FLAGS += --platform=node
TEST_ROLLUP_FLAGS += --target=es6
TEST_ROLLUP_FLAGS += src/node-entry.ts

github/rollup:
	mkdir -p github
	git clone --depth 1 --branch v2.60.2 https://github.com/rollup/rollup.git github/rollup

demo/rollup: | github/rollup
	mkdir -p demo
	cp -RP github/rollup/ demo/rollup
	cd demo/rollup && npm ci

	# Patch over Rollup's custom "package.json" alias using "tsconfig.json"
	cat demo/rollup/tsconfig.json | sed 's/$(TEST_ROLLUP_FIND)/$(TEST_ROLLUP_REPLACE)/' > demo/rollup/tsconfig2.json
	mv demo/rollup/tsconfig2.json demo/rollup/tsconfig.json

test-rollup: esbuild | demo/rollup
	# Skip watch tests to avoid flakes
	cd demo/rollup && ../../esbuild $(TEST_ROLLUP_FLAGS) && npm run test:only -- --fgrep watch --invert
	cd demo/rollup && ../../esbuild $(TEST_ROLLUP_FLAGS) --minify && npm run test:only -- --fgrep watch --invert

################################################################################
# This builds Preact using esbuild with splitting enabled, which had a bug at one point

PREACT_SPLITTING += import { h } from 'preact';
PREACT_SPLITTING += import { USE as use } from 'preact/hooks';
PREACT_SPLITTING += import { renderToString } from 'preact-render-to-string';
PREACT_SPLITTING += let Component = () => (use(() => {}), h('div'));
PREACT_SPLITTING += if (renderToString(h(Component)) !== '<div></div>') throw 'fail';

PREACT_HOOKS += useCallback
PREACT_HOOKS += useContext
PREACT_HOOKS += useDebugValue
PREACT_HOOKS += useEffect
PREACT_HOOKS += useErrorBoundary
PREACT_HOOKS += useImperativeHandle
PREACT_HOOKS += useLayoutEffect
PREACT_HOOKS += useMemo
PREACT_HOOKS += useReducer
PREACT_HOOKS += useRef
PREACT_HOOKS += useState

demo/preact-splitting:
	mkdir -p demo/preact-splitting/src
	cd demo/preact-splitting && echo '{}' > package.json && npm i preact@10.4.6 preact-render-to-string@5.1.10
	cd demo/preact-splitting && for h in $(PREACT_HOOKS); do echo "$(PREACT_SPLITTING)" | sed s/USE/$$h/ > src/$$h.js; done

test-preact-splitting: esbuild | demo/preact-splitting
	cd demo/preact-splitting && rm -fr out && ../../esbuild --bundle --splitting --format=esm src/*.js --outdir=out --out-extension:.js=.mjs
	cd demo/preact-splitting && for h in $(PREACT_HOOKS); do set -e && node --experimental-modules out/$$h.mjs; done
	cd demo/preact-splitting && rm -fr out && ../../esbuild --bundle --splitting --format=esm src/*.js --outdir=out --out-extension:.js=.mjs --minify --target=node12
	cd demo/preact-splitting && for h in $(PREACT_HOOKS); do set -e && node --experimental-modules out/$$h.mjs; done

################################################################################
# This builds Sucrase using esbuild and then uses it to run Sucrase's test suite

github/sucrase:
	mkdir -p github/sucrase
	cd github/sucrase && git init && git remote add origin https://github.com/alangpierce/sucrase.git
	cd github/sucrase && git fetch --depth 1 origin a4a596e5cdd57362f309ae50cc32a235d7817d34 && git checkout FETCH_HEAD

demo/sucrase: | github/sucrase
	mkdir -p demo
	cp -r github/sucrase/ demo/sucrase
	cd demo/sucrase && npm i
	cd demo/sucrase && find test -name '*.ts' | sed 's/\(.*\)\.ts/import ".\/\1"/g' > all-tests.ts
	echo '{}' > demo/sucrase/tsconfig.json # Sucrase tests fail if tsconfig.json is respected due to useDefineForClassFields

test-sucrase: esbuild | demo/sucrase
	cd demo/sucrase && ../../esbuild --bundle all-tests.ts --target=es6 --platform=node > out.js && npx mocha out.js
	cd demo/sucrase && ../../esbuild --bundle all-tests.ts --target=es6 --platform=node --minify > out.js && npx mocha out.js

################################################################################
# This builds Esprima using esbuild and then uses it to run Esprima's test suite

github/esprima:
	mkdir -p github/esprima
	cd github/esprima && git init && git remote add origin https://github.com/jquery/esprima.git
	cd github/esprima && git fetch --depth 1 origin fa49b2edc288452eb49441054ce6f7ff4b891eb4 && git checkout FETCH_HEAD

demo/esprima: | github/esprima
	mkdir -p demo
	cp -r github/esprima/ demo/esprima
	cd demo/esprima && npm ci

test-esprima: esbuild | demo/esprima
	cd demo/esprima && ../../esbuild --bundle src/esprima.ts --outfile=dist/esprima.js --target=es6 --platform=node && npm run all-tests
	cd demo/esprima && ../../esbuild --bundle src/esprima.ts --outfile=dist/esprima.js --target=es6 --platform=node --minify && npm run all-tests

################################################################################
# This runs terser's test suite through esbuild

github/terser:
	mkdir -p github/terser
	cd github/terser && git init && git remote add origin https://github.com/terser/terser.git
	cd github/terser && git fetch --depth 1 origin 056623c20dbbc42d2f5a34926c07133981519326 && git checkout FETCH_HEAD

demo/terser: | github/terser
	mkdir -p demo
	cp -r github/terser/ demo/terser
	cd demo/terser && npm ci && npm run build

terser: esbuild | demo/terser
	node scripts/terser-tests.js

################################################################################
# three.js demo

github/three:
	mkdir -p github
	git clone --depth 1 --branch r108 https://github.com/mrdoob/three.js.git github/three

demo/three: | github/three
	mkdir -p demo/three
	cp -r github/three/src demo/three/src

demo-three: demo-three-esbuild demo-three-rollup demo-three-webpack5 demo-three-parcel2

demo-three-esbuild: esbuild | demo/three
	rm -fr demo/three/esbuild
	time -p ./esbuild --bundle --global-name=THREE --sourcemap --minify demo/three/src/Three.js --outfile=demo/three/esbuild/Three.esbuild.js
	du -h demo/three/esbuild/Three.esbuild.js*
	shasum demo/three/esbuild/Three.esbuild.js*

demo-three-eswasm: platform-wasm | demo/three
	rm -fr demo/three/eswasm
	time -p ./npm/esbuild-wasm/bin/esbuild --bundle --global-name=THREE \
		--sourcemap --minify demo/three/src/Three.js --outfile=demo/three/eswasm/Three.eswasm.js
	du -h demo/three/eswasm/Three.eswasm.js*
	shasum demo/three/eswasm/Three.eswasm.js*

THREE_ROLLUP_CONFIG += import terser from '@rollup/plugin-terser';
THREE_ROLLUP_CONFIG += export default {
THREE_ROLLUP_CONFIG +=   output: { format: 'iife', name: 'THREE', sourcemap: true },
THREE_ROLLUP_CONFIG +=   plugins: [terser()],
THREE_ROLLUP_CONFIG += }

demo-three-rollup: | require/rollup/node_modules demo/three
	rm -fr require/rollup/demo/three demo/three/rollup
	mkdir -p require/rollup/demo/three demo/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > require/rollup/demo/three/config.mjs
	ln -s ../../../../demo/three/src require/rollup/demo/three/src
	ln -s ../../../../demo/three/rollup require/rollup/demo/three/out
	cd require/rollup/demo/three && time -p ../../node_modules/.bin/rollup src/Three.js -o out/Three.rollup.js -c config.mjs
	du -h demo/three/rollup/Three.rollup.js*

THREE_WEBPACK5_FLAGS += --devtool=source-map
THREE_WEBPACK5_FLAGS += --mode=production
THREE_WEBPACK5_FLAGS += --output-library THREE

demo-three-webpack5: | require/webpack5/node_modules demo/three
	rm -fr require/webpack5/demo/three demo/three/webpack5
	mkdir -p require/webpack5/demo/three demo/three/webpack5
	ln -s ../../../../demo/three/src require/webpack5/demo/three/src
	ln -s ../../../../demo/three/webpack5 require/webpack5/demo/three/out
	cd require/webpack5/demo/three && time -p ../../node_modules/.bin/webpack --entry ./src/Three.js $(THREE_WEBPACK5_FLAGS) -o out/Three.webpack5.js
	du -h demo/three/webpack5/Three.webpack5.js*

demo-three-parcel2: | require/parcel2/node_modules demo/three
	rm -fr require/parcel2/demo/three demo/three/parcel2
	mkdir -p require/parcel2/demo/three demo/three/parcel2

	# Copy the whole source tree since symlinks mess up Parcel's internal package lookup for "@babel/core"
	cp -r demo/three/src require/parcel2/demo/three/src

	echo 'import * as THREE from "./src/Three.js"; window.THREE = THREE' > require/parcel2/demo/three/Three.parcel2.js
	cd require/parcel2/demo/three && time -p ../../node_modules/.bin/parcel build \
		Three.parcel2.js --dist-dir ../../../../demo/three/parcel2 --cache-dir .cache
	du -h demo/three/parcel2/Three.parcel2.js*

################################################################################
# three.js benchmark (measures JavaScript performance, same as three.js demo but 10x bigger)

bench/three: | github/three
	mkdir -p bench/three/src
	echo > bench/three/src/entry.js
	for i in 1 2 3 4 5 6 7 8 9 10; do test -d "bench/three/src/copy$$i" || cp -r github/three/src "bench/three/src/copy$$i"; done
	for i in 1 2 3 4 5 6 7 8 9 10; do echo "import * as copy$$i from './copy$$i/Three.js'; export {copy$$i}" >> bench/three/src/entry.js; done
	echo 'Line count:' && find bench/three/src -name '*.js' | xargs wc -l | tail -n 1

bench-three: bench-three-esbuild bench-three-rollup bench-three-webpack5 bench-three-parcel2

bench-three-esbuild: esbuild | bench/three
	rm -fr bench/three/esbuild
	time -p ./esbuild --bundle --global-name=THREE --sourcemap --minify bench/three/src/entry.js --outfile=bench/three/esbuild/entry.esbuild.js --timing
	du -h bench/three/esbuild/entry.esbuild.js*
	shasum bench/three/esbuild/entry.esbuild.js*

bench-three-eswasm: platform-wasm | bench/three
	rm -fr bench/three/eswasm
	time -p ./npm/esbuild-wasm/bin/esbuild --bundle --global-name=THREE \
		--sourcemap --minify bench/three/src/entry.js --outfile=bench/three/eswasm/entry.eswasm.js
	du -h bench/three/eswasm/entry.eswasm.js*
	shasum bench/three/eswasm/entry.eswasm.js*

bench-three-rollup: | require/rollup/node_modules bench/three
	rm -fr require/rollup/bench/three bench/three/rollup
	mkdir -p require/rollup/bench/three bench/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > require/rollup/bench/three/config.mjs
	ln -s ../../../../bench/three/src require/rollup/bench/three/src
	ln -s ../../../../bench/three/rollup require/rollup/bench/three/out
	cd require/rollup/bench/three && time -p ../../node_modules/.bin/rollup src/entry.js -o out/entry.rollup.js -c config.mjs
	du -h bench/three/rollup/entry.rollup.js*

bench-three-webpack5: | require/webpack5/node_modules bench/three
	rm -fr require/webpack5/bench/three bench/three/webpack5
	mkdir -p require/webpack5/bench/three bench/three/webpack5
	ln -s ../../../../bench/three/src require/webpack5/bench/three/src
	ln -s ../../../../bench/three/webpack5 require/webpack5/bench/three/out
	cd require/webpack5/bench/three && time -p ../../node_modules/.bin/webpack --entry ./src/entry.js $(THREE_WEBPACK5_FLAGS) -o out/entry.webpack5.js
	du -h bench/three/webpack5/entry.webpack5.js*

bench-three-parcel2: | require/parcel2/node_modules bench/three
	rm -fr require/parcel2/bench/three bench/three/parcel2
	mkdir -p require/parcel2/bench/three bench/three/parcel2

	# Copy the whole source tree since symlinks mess up Parcel's internal package lookup for "@babel/core"
	cp -r bench/three/src require/parcel2/bench/three/src

	echo 'import * as THREE from "./src/entry.js"; window.THREE = THREE' > require/parcel2/bench/three/entry.parcel2.js
	cd require/parcel2/bench/three && time -p node ../../node_modules/.bin/parcel build \
		entry.parcel2.js --dist-dir ../../../../bench/three/parcel2 --cache-dir .cache
	du -h bench/three/parcel2/entry.parcel2.js*

################################################################################
# Rome benchmark (measures TypeScript performance)

ROME_TSCONFIG += {
ROME_TSCONFIG +=   \"compilerOptions\": {
ROME_TSCONFIG +=     \"sourceMap\": true,
ROME_TSCONFIG +=     \"esModuleInterop\": true,
ROME_TSCONFIG +=     \"resolveJsonModule\": true,
ROME_TSCONFIG +=     \"moduleResolution\": \"node\",
ROME_TSCONFIG +=     \"target\": \"es2019\",
ROME_TSCONFIG +=     \"module\": \"commonjs\",
ROME_TSCONFIG +=     \"baseUrl\": \".\"
ROME_TSCONFIG +=   }
ROME_TSCONFIG += }

github/rome:
	mkdir -p github/rome
	cd github/rome && git init && git remote add origin https://github.com/romejs/rome.git
	cd github/rome && git fetch --depth 1 origin d95a3a7aab90773c9b36d9c82a08c8c4c6b68aa5 && git checkout FETCH_HEAD

bench/rome: | github/rome
	mkdir -p bench/rome
	cp -r github/rome/packages bench/rome/src
	echo "$(ROME_TSCONFIG)" > bench/rome/src/tsconfig.json
	echo 'import "rome/bin/rome"' > bench/rome/src/entry.ts

	# Patch a cyclic import ordering issue that affects commonjs-style bundlers (webpack and parcel)
	echo "export { default as createHook } from './api/createHook';" > .temp
	sed "/createHook/d" bench/rome/src/@romejs/js-compiler/index.ts >> .temp
	mv .temp bench/rome/src/@romejs/js-compiler/index.ts

	# Replace "import fs = require('fs')" with "const fs = require('fs')" because
	# the TypeScript compiler strips these statements when targeting "esnext",
	# which breaks Parcel 2 when scope hoisting is enabled.
	find bench/rome/src -name '*.ts' -type f -print0 | xargs -L1 -0 sed -i '' 's/import \([A-Za-z0-9_]*\) =/const \1 =/g'
	find bench/rome/src -name '*.tsx' -type f -print0 | xargs -L1 -0 sed -i '' 's/import \([A-Za-z0-9_]*\) =/const \1 =/g'

	# Get an approximate line count
	rm -r bench/rome/src/@romejs/js-parser/test-fixtures
	echo 'Line count:' && (find bench/rome/src -name '*.ts' && find bench/rome/src -name '*.js') | xargs wc -l | tail -n 1

# This target provides an easy way to verify that the build is correct. Since
# Rome is self-hosted, we can just run the bundle to build Rome. This makes sure
# the bundle doesn't crash when run and is a good test of a non-trivial workload.
bench/rome-verify: | github/rome
	mkdir -p bench/rome-verify
	cp -r github/rome/packages bench/rome-verify/packages
	cp github/rome/package.json bench/rome-verify/package.json

bench-rome: bench-rome-esbuild bench-rome-webpack5 bench-rome-parcel2

bench-rome-esbuild: esbuild | bench/rome bench/rome-verify
	rm -fr bench/rome/esbuild
	time -p ./esbuild --bundle --sourcemap --minify bench/rome/src/entry.ts --outfile=bench/rome/esbuild/rome.esbuild.js --platform=node --timing
	time -p ./esbuild --bundle --sourcemap --minify bench/rome/src/entry.ts --outfile=bench/rome/esbuild/rome.esbuild.js --platform=node --timing
	time -p ./esbuild --bundle --sourcemap --minify bench/rome/src/entry.ts --outfile=bench/rome/esbuild/rome.esbuild.js --platform=node --timing
	du -h bench/rome/esbuild/rome.esbuild.js*
	shasum bench/rome/esbuild/rome.esbuild.js*
	cd bench/rome-verify && rm -fr esbuild && ROME_CACHE=0 node ../rome/esbuild/rome.esbuild.js bundle packages/rome esbuild

ROME_WEBPACK5_CONFIG += module.exports = {
ROME_WEBPACK5_CONFIG +=   entry: './src/entry.ts',
ROME_WEBPACK5_CONFIG +=   mode: 'production',
ROME_WEBPACK5_CONFIG +=   target: 'node',
ROME_WEBPACK5_CONFIG +=   devtool: 'source-map',
ROME_WEBPACK5_CONFIG +=   module: { rules: [{ test: /\.ts$$/, loader: 'ts-loader', options: { transpileOnly: true } }] },
ROME_WEBPACK5_CONFIG +=   resolve: {
ROME_WEBPACK5_CONFIG +=     extensions: ['.ts', '.js'],
ROME_WEBPACK5_CONFIG +=     alias: { rome: __dirname + '/src/rome', '@romejs': __dirname + '/src/@romejs' },
ROME_WEBPACK5_CONFIG +=   },
ROME_WEBPACK5_CONFIG +=   output: { filename: 'rome.webpack.js', path: __dirname + '/out' },
ROME_WEBPACK5_CONFIG += };

bench-rome-webpack5: | require/webpack5/node_modules bench/rome bench/rome-verify
	rm -fr require/webpack5/bench/rome bench/rome/webpack5
	mkdir -p require/webpack5/bench/rome bench/rome/webpack5
	echo "$(ROME_WEBPACK5_CONFIG)" > require/webpack5/bench/rome/webpack.config.js
	ln -s ../../../../bench/rome/src require/webpack5/bench/rome/src
	ln -s ../../../../bench/rome/webpack5 require/webpack5/bench/rome/out
	cd require/webpack5/bench/rome && time -p ../../node_modules/.bin/webpack
	du -h bench/rome/webpack5/rome.webpack.js*
	cd bench/rome-verify && rm -fr webpack5 && ROME_CACHE=0 node ../rome/webpack5/rome.webpack.js bundle packages/rome webpack5

ROME_PARCEL_ALIASES += "alias": {
ROME_PARCEL_ALIASES +=   $(shell ls bench/rome/src/@romejs | sed 's/.*/"\@romejs\/&": ".\/@romejs\/&",/g')
ROME_PARCEL_ALIASES +=   "rome": "./rome"
ROME_PARCEL_ALIASES += }

bench-rome-parcel2: | require/parcel2/node_modules bench/rome bench/rome-verify
	rm -fr bench/rome/parcel2
	cp -r bench/rome/src bench/rome/parcel2
	rm -fr bench/rome/parcel2/node_modules
	cp -RP require/parcel2/node_modules bench/rome/parcel2/node_modules

	# Inject aliases into "package.json" to fix Parcel 2 ignoring "tsconfig.json".
	# Also inject "engines": "node" to avoid Parcel 2 mangling node globals.
	cat require/parcel2/package.json | sed '/^\}/d' > bench/rome/parcel2/package.json
	echo ', "engines": { "node": "14.0.0" }' >> bench/rome/parcel2/package.json
	echo ', $(ROME_PARCEL_ALIASES) }' >> bench/rome/parcel2/package.json

	cd bench/rome/parcel2 && time -p node_modules/.bin/parcel build entry.ts --dist-dir . --cache-dir .cache
	du -h bench/rome/parcel2/entry.js*
	cd bench/rome-verify && rm -fr parcel2 && ROME_CACHE=0 node ../rome/parcel2/entry.js bundle packages/rome parcel2

################################################################################
# React admin benchmark (measures performance of an application-like setup)

READMIN_HTML = <meta charset=utf8><div id=root></div><script src=index.js type=module></script>

github/react-admin:
	mkdir -p github
	git clone --depth 1 --branch v4.6.1 https://github.com/marmelab/react-admin.git github/react-admin

bench/readmin: | github/react-admin
	mkdir -p bench/readmin
	cp -r github/react-admin bench/readmin/repo
	cd bench/readmin/repo && yarn # This takes approximately forever

bench-readmin: bench-readmin-esbuild

READMIN_ESBUILD_FLAGS += --alias:data-generator-retail=./bench/readmin/repo/examples/data-generator/src
READMIN_ESBUILD_FLAGS += --alias:ra-core=./bench/readmin/repo/packages/ra-core/src
READMIN_ESBUILD_FLAGS += --alias:ra-data-fakerest=./bench/readmin/repo/packages/ra-data-fakerest/src
READMIN_ESBUILD_FLAGS += --alias:ra-data-graphql-simple=./bench/readmin/repo/packages/ra-data-graphql-simple/src
READMIN_ESBUILD_FLAGS += --alias:ra-data-graphql=./bench/readmin/repo/packages/ra-data-graphql/src
READMIN_ESBUILD_FLAGS += --alias:ra-data-simple-rest=./bench/readmin/repo/packages/ra-data-simple-rest/src
READMIN_ESBUILD_FLAGS += --alias:ra-i18n-polyglot=./bench/readmin/repo/packages/ra-i18n-polyglot/src
READMIN_ESBUILD_FLAGS += --alias:ra-input-rich-text=./bench/readmin/repo/packages/ra-input-rich-text/src
READMIN_ESBUILD_FLAGS += --alias:ra-language-english=./bench/readmin/repo/packages/ra-language-english/src
READMIN_ESBUILD_FLAGS += --alias:ra-language-french=./bench/readmin/repo/packages/ra-language-french/src
READMIN_ESBUILD_FLAGS += --alias:ra-ui-materialui=./bench/readmin/repo/packages/ra-ui-materialui/src
READMIN_ESBUILD_FLAGS += --alias:react-admin=./bench/readmin/repo/packages/react-admin/src
READMIN_ESBUILD_FLAGS += --bundle
READMIN_ESBUILD_FLAGS += --define:process.env.REACT_APP_DATA_PROVIDER=null
READMIN_ESBUILD_FLAGS += --format=esm
READMIN_ESBUILD_FLAGS += --loader:.png=file
READMIN_ESBUILD_FLAGS += --loader:.svg=file
READMIN_ESBUILD_FLAGS += --minify
READMIN_ESBUILD_FLAGS += --sourcemap
READMIN_ESBUILD_FLAGS += --splitting
READMIN_ESBUILD_FLAGS += --target=esnext
READMIN_ESBUILD_FLAGS += --timing
READMIN_ESBUILD_FLAGS += bench/readmin/repo/examples/demo/src/index.tsx

bench-readmin-esbuild: esbuild | bench/readmin
	rm -fr bench/readmin/esbuild
	time -p ./esbuild $(READMIN_ESBUILD_FLAGS) --outdir=bench/readmin/esbuild
	echo "$(READMIN_HTML)" > bench/readmin/esbuild/index.html
	du -h bench/readmin/esbuild/index.js*
	shasum bench/readmin/esbuild/index.js*

bench-readmin-eswasm: platform-wasm | bench/readmin
	rm -fr bench/readmin/eswasm
	time -p ./npm/esbuild-wasm/bin/esbuild $(READMIN_ESBUILD_FLAGS) --outdir=bench/readmin/eswasm
	echo "$(READMIN_HTML)" > bench/readmin/eswasm/index.html
	du -h bench/readmin/eswasm/index.js*
	shasum bench/readmin/eswasm/index.js*
