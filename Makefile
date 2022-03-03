ESBUILD_VERSION = $(shell cat version.txt)

# Strip debug info
GO_FLAGS += "-ldflags=-s -w"

# Avoid embedding the build path in the executable for more reproducible builds
GO_FLAGS += -trimpath

# Temporary workaround for https://github.com/golang/go/issues/51101 before Go 1.18/1.17.8 is released
ifeq ($(GOARCH), riscv64)
	GO_FLAGS += "-gcflags=all=-N -l"
endif

esbuild: cmd/esbuild/version.go cmd/esbuild/*.go pkg/*/*.go internal/*/*.go go.mod
	CGO_ENABLED=0 go build $(GO_FLAGS) ./cmd/esbuild

test:
	@$(MAKE) --no-print-directory -j6 test-common

# These tests are for development
test-common: test-go vet-go no-filepath verify-source-map end-to-end-tests js-api-tests plugin-tests register-test node-unref-tests

# These tests are for release (the extra tests are not included in "test" because they are pretty slow)
test-all:
	@$(MAKE) --no-print-directory -j6 test-common test-deno ts-type-tests test-wasm-node test-wasm-browser lib-typecheck

check-go-version:
	@go version | grep ' go1\.17\.7 ' || (echo 'Please install Go version 1.17.7' && false)

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

register-test: cmd/esbuild/version.go | scripts/node_modules
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/register-test.js

verify-source-map: cmd/esbuild/version.go | scripts/node_modules
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/verify-source-map.js

end-to-end-tests: cmd/esbuild/version.go | scripts/node_modules
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/end-to-end-tests.js

js-api-tests: cmd/esbuild/version.go | scripts/node_modules
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/js-api-tests.js

plugin-tests: cmd/esbuild/version.go | scripts/node_modules
	node scripts/plugin-tests.js

ts-type-tests: | scripts/node_modules
	node scripts/ts-type-tests.js

node-unref-tests: | scripts/node_modules
	node scripts/node-unref-tests.js

lib-typecheck: | lib/node_modules
	cd lib && node_modules/.bin/tsc -noEmit -p tsconfig.json
	cd lib && node_modules/.bin/tsc -noEmit -p tsconfig-deno.json

# End-to-end tests
test-e2e: test-e2e-npm test-e2e-pnpm test-e2e-yarn-berry

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
	rm -fr e2e-yb && mkdir e2e-yb && cd e2e-yb && echo {} > package.json && echo 'enableScripts: false' > yarn.lock && yarn set version berry && yarn add esbuild
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

cmd/esbuild/version.go: version.txt
	node scripts/esbuild.js --update-version-go

wasm-napi-exit0-darwin:
	node -e 'console.log(`#include <unistd.h>\nvoid* napi_register_module_v1(void* a, void* b) { _exit(0); }`)' \
		| clang -x c -dynamiclib -mmacosx-version-min=10.5 -o lib/npm/exit0/darwin-x64-LE.node -
	ls -l lib/npm/exit0/darwin-x64-LE.node

wasm-napi-exit0-darwin-arm:
	node -e 'console.log(`#include <unistd.h>\nvoid* napi_register_module_v1(void* a, void* b) { _exit(0); }`)' \
		| clang -x c -dynamiclib -mmacosx-version-min=10.5 -o lib/npm/exit0/darwin-arm64-LE.node -
	ls -l lib/npm/exit0/darwin-arm64-LE.node

wasm-napi-exit0-linux:
	node -e 'console.log(`#include <unistd.h>\nvoid* napi_register_module_v1(void* a, void* b) { _exit(0); }`)' \
		| gcc -x c -shared -o lib/npm/exit0/linux-x64-LE.node -
	strip lib/npm/exit0/linux-x64-LE.node
	ls -l lib/npm/exit0/linux-x64-LE.node

wasm-napi-exit0-linux-arm:
	node -e 'console.log(`#include <unistd.h>\nvoid* napi_register_module_v1(void* a, void* b) { _exit(0); }`)' \
		| gcc -x c -shared -o lib/npm/exit0/linux-arm64-LE.node -
	strip lib/npm/exit0/linux-arm64-LE.node
	ls -l lib/npm/exit0/linux-arm64-LE.node

wasm-napi-exit0-windows:
	# This isn't meant to be run directly but is a rough overview of the instructions
	echo '__declspec(dllexport) void* napi_register_module_v1(void* a, void* b) { ExitProcess(0); }' > main.c
	echo 'setlocal' > main.bat
	echo 'call "C:\Program Files (x86)\Microsoft Visual Studio\2019\Community\VC\Auxiliary\Build\vcvarsall.bat" x64' >> main.bat
	echo 'cl.exe /LD main.c /link /DLL /NODEFAULTLIB /NOENTRY kernel32.lib /OUT:lib/npm/exit0/win32-x64-LE.node' >> main.bat
	main.bat
	rm -f main.*

platform-all:
	@$(MAKE) --no-print-directory -j4 \
		platform-android \
		platform-android-arm64 \
		platform-darwin \
		platform-darwin-arm64 \
		platform-deno \
		platform-freebsd \
		platform-freebsd-arm64 \
		platform-linux \
		platform-linux-32 \
		platform-linux-arm \
		platform-linux-arm64 \
		platform-linux-mips64le \
		platform-linux-ppc64le \
		platform-linux-riscv64 \
		platform-linux-s390x \
		platform-netbsd \
		platform-neutral \
		platform-openbsd \
		platform-sunos \
		platform-wasm \
		platform-windows \
		platform-windows-32 \
		platform-windows-arm64

platform-windows: cmd/esbuild/version.go
	node scripts/esbuild.js npm/esbuild-windows-64/package.json --version
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GO_FLAGS) -o npm/esbuild-windows-64/esbuild.exe ./cmd/esbuild

platform-windows-32: cmd/esbuild/version.go
	node scripts/esbuild.js npm/esbuild-windows-32/package.json --version
	CGO_ENABLED=0 GOOS=windows GOARCH=386 go build $(GO_FLAGS) -o npm/esbuild-windows-32/esbuild.exe ./cmd/esbuild

platform-windows-arm64: cmd/esbuild/version.go
	node scripts/esbuild.js npm/esbuild-windows-arm64/package.json --version
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(GO_FLAGS) -o npm/esbuild-windows-arm64/esbuild.exe ./cmd/esbuild

platform-unixlike: cmd/esbuild/version.go
	@test -n "$(GOOS)" || (echo "The environment variable GOOS must be provided" && false)
	@test -n "$(GOARCH)" || (echo "The environment variable GOARCH must be provided" && false)
	@test -n "$(NPMDIR)" || (echo "The environment variable NPMDIR must be provided" && false)
	node scripts/esbuild.js "$(NPMDIR)/package.json" --version
	CGO_ENABLED=0 GOOS="$(GOOS)" GOARCH="$(GOARCH)" go build $(GO_FLAGS) -o "$(NPMDIR)/bin/esbuild" ./cmd/esbuild

platform-android: platform-wasm
	node scripts/esbuild.js npm/esbuild-android-64/package.json --version

platform-android-arm64:
	@$(MAKE) --no-print-directory GOOS=android GOARCH=arm64 NPMDIR=npm/esbuild-android-arm64 platform-unixlike

platform-darwin:
	@$(MAKE) --no-print-directory GOOS=darwin GOARCH=amd64 NPMDIR=npm/esbuild-darwin-64 platform-unixlike

platform-darwin-arm64:
	@$(MAKE) --no-print-directory GOOS=darwin GOARCH=arm64 NPMDIR=npm/esbuild-darwin-arm64 platform-unixlike

platform-freebsd:
	@$(MAKE) --no-print-directory GOOS=freebsd GOARCH=amd64 NPMDIR=npm/esbuild-freebsd-64 platform-unixlike

platform-freebsd-arm64:
	@$(MAKE) --no-print-directory GOOS=freebsd GOARCH=arm64 NPMDIR=npm/esbuild-freebsd-arm64 platform-unixlike

platform-netbsd:
	@$(MAKE) --no-print-directory GOOS=netbsd GOARCH=amd64 NPMDIR=npm/esbuild-netbsd-64 platform-unixlike

platform-openbsd:
	@$(MAKE) --no-print-directory GOOS=openbsd GOARCH=amd64 NPMDIR=npm/esbuild-openbsd-64 platform-unixlike

platform-linux:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=amd64 NPMDIR=npm/esbuild-linux-64 platform-unixlike

platform-linux-32:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=386 NPMDIR=npm/esbuild-linux-32 platform-unixlike

platform-linux-arm:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=arm NPMDIR=npm/esbuild-linux-arm platform-unixlike

platform-linux-arm64:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=arm64 NPMDIR=npm/esbuild-linux-arm64 platform-unixlike

platform-linux-mips64le:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=mips64le NPMDIR=npm/esbuild-linux-mips64le platform-unixlike

platform-linux-ppc64le:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=ppc64le NPMDIR=npm/esbuild-linux-ppc64le platform-unixlike

platform-linux-riscv64:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=riscv64 NPMDIR=npm/esbuild-linux-riscv64 platform-unixlike

platform-linux-s390x:
	@$(MAKE) --no-print-directory GOOS=linux GOARCH=s390x NPMDIR=npm/esbuild-linux-s390x platform-unixlike

platform-sunos:
	@$(MAKE) --no-print-directory GOOS=illumos GOARCH=amd64 NPMDIR=npm/esbuild-sunos-64 platform-unixlike

platform-wasm: esbuild
	node scripts/esbuild.js npm/esbuild-wasm/package.json --version
	node scripts/esbuild.js ./esbuild --wasm

platform-neutral: esbuild
	node scripts/esbuild.js npm/esbuild/package.json --version
	node scripts/esbuild.js ./esbuild --neutral

platform-deno: esbuild
	node scripts/esbuild.js ./esbuild --deno

publish-all: check-go-version
	@npm --version > /dev/null || (echo "The 'npm' command must be in your path to publish" && false)
	@echo "Checking for uncommitted/untracked changes..." && test -z "`git status --porcelain | grep -vE 'M (CHANGELOG\.md|version\.txt)'`" || \
		(echo "Refusing to publish with these uncommitted/untracked changes:" && \
		git status --porcelain | grep -vE 'M (CHANGELOG\.md|version\.txt)' && false)
	@echo "Checking for master branch..." && test master = "`git rev-parse --abbrev-ref HEAD`" || \
		(echo "Refusing to publish from non-master branch `git rev-parse --abbrev-ref HEAD`" && false)
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
		publish-windows \
		publish-windows-32 \
		publish-windows-arm64 \
		publish-sunos

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-freebsd \
		publish-freebsd-arm64 \
		publish-openbsd \
		publish-netbsd

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-android \
		publish-android-arm64 \
		publish-darwin \
		publish-darwin-arm64

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-linux \
		publish-linux-32 \
		publish-linux-arm \
		publish-linux-riscv64

	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-linux-arm64 \
		publish-linux-mips64le \
		publish-linux-ppc64le \
		publish-linux-s390x

	# Do these last to avoid race conditions
	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" $(MAKE) --no-print-directory -j4 \
		publish-neutral \
		publish-deno \
		publish-wasm

	git push origin master "v$(ESBUILD_VERSION)"

publish-windows: platform-windows
	test -n "$(OTP)" && cd npm/esbuild-windows-64 && npm publish --otp="$(OTP)"

publish-windows-32: platform-windows-32
	test -n "$(OTP)" && cd npm/esbuild-windows-32 && npm publish --otp="$(OTP)"

publish-windows-arm64: platform-windows-arm64
	test -n "$(OTP)" && cd npm/esbuild-windows-arm64 && npm publish --otp="$(OTP)"

publish-android: platform-android
	test -n "$(OTP)" && cd npm/esbuild-android-64 && npm publish --otp="$(OTP)"

publish-android-arm64: platform-android-arm64
	test -n "$(OTP)" && cd npm/esbuild-android-arm64 && npm publish --otp="$(OTP)"

publish-darwin: platform-darwin
	test -n "$(OTP)" && cd npm/esbuild-darwin-64 && npm publish --otp="$(OTP)"

publish-darwin-arm64: platform-darwin-arm64
	test -n "$(OTP)" && cd npm/esbuild-darwin-arm64 && npm publish --otp="$(OTP)"

publish-freebsd: platform-freebsd
	test -n "$(OTP)" && cd npm/esbuild-freebsd-64 && npm publish --otp="$(OTP)"

publish-freebsd-arm64: platform-freebsd-arm64
	test -n "$(OTP)" && cd npm/esbuild-freebsd-arm64 && npm publish --otp="$(OTP)"

publish-netbsd: platform-netbsd
	test -n "$(OTP)" && cd npm/esbuild-netbsd-64 && npm publish --otp="$(OTP)"

publish-openbsd: platform-openbsd
	test -n "$(OTP)" && cd npm/esbuild-openbsd-64 && npm publish --otp="$(OTP)"

publish-linux: platform-linux
	test -n "$(OTP)" && cd npm/esbuild-linux-64 && npm publish --otp="$(OTP)"

publish-linux-32: platform-linux-32
	test -n "$(OTP)" && cd npm/esbuild-linux-32 && npm publish --otp="$(OTP)"

publish-linux-arm: platform-linux-arm
	test -n "$(OTP)" && cd npm/esbuild-linux-arm && npm publish --otp="$(OTP)"

publish-linux-arm64: platform-linux-arm64
	test -n "$(OTP)" && cd npm/esbuild-linux-arm64 && npm publish --otp="$(OTP)"

publish-linux-mips64le: platform-linux-mips64le
	test -n "$(OTP)" && cd npm/esbuild-linux-mips64le && npm publish --otp="$(OTP)"

publish-linux-ppc64le: platform-linux-ppc64le
	test -n "$(OTP)" && cd npm/esbuild-linux-ppc64le && npm publish --otp="$(OTP)"

publish-linux-riscv64: platform-linux-riscv64
	test -n "$(OTP)" && cd npm/esbuild-linux-riscv64 && npm publish --otp="$(OTP)"

publish-linux-s390x: platform-linux-s390x
	test -n "$(OTP)" && cd npm/esbuild-linux-s390x && npm publish --otp="$(OTP)"

publish-sunos: platform-sunos
	test -n "$(OTP)" && cd npm/esbuild-sunos-64 && npm publish --otp="$(OTP)"

publish-wasm: platform-wasm
	test -n "$(OTP)" && cd npm/esbuild-wasm && npm publish --otp="$(OTP)"

publish-neutral: platform-neutral
	test -n "$(OTP)" && cd npm/esbuild && npm publish --otp="$(OTP)"

publish-deno:
	test -d deno/.git || (rm -fr deno && git clone git@github.com:esbuild/deno-esbuild.git deno)
	cd deno && git fetch && git checkout main && git reset --hard origin/main
	@$(MAKE) --no-print-directory platform-deno
	cd deno && git commit -am "publish $(ESBUILD_VERSION) to deno"
	cd deno && git tag "v$(ESBUILD_VERSION)"
	cd deno && git push origin main "v$(ESBUILD_VERSION)"

clean:
	rm -f esbuild
	rm -f npm/esbuild-windows-32/esbuild.exe
	rm -f npm/esbuild-windows-64/esbuild.exe
	rm -f npm/esbuild-windows-arm64/esbuild.exe
	rm -rf npm/esbuild-android-64/bin
	rm -rf npm/esbuild-android-64/esbuild.wasm npm/esbuild-android-64/wasm_exec.js npm/esbuild-android-64/exit0.js
	rm -rf npm/esbuild-android-arm64/bin
	rm -rf npm/esbuild-darwin-64/bin
	rm -rf npm/esbuild-darwin-arm64/bin
	rm -rf npm/esbuild-freebsd-64/bin
	rm -rf npm/esbuild-freebsd-amd64/bin
	rm -rf npm/esbuild-linux-32/bin
	rm -rf npm/esbuild-linux-64/bin
	rm -rf npm/esbuild-linux-arm/bin
	rm -rf npm/esbuild-linux-arm64/bin
	rm -rf npm/esbuild-linux-mips64le/bin
	rm -rf npm/esbuild-linux-ppc64le/bin
	rm -rf npm/esbuild-linux-riscv64/bin
	rm -rf npm/esbuild-linux-s390x/bin
	rm -rf npm/esbuild-netbsd-64/bin
	rm -rf npm/esbuild-openbsd-64/bin
	rm -rf npm/esbuild-sunos-64/bin
	rm -rf npm/esbuild/bin
	rm -f npm/esbuild-wasm/esbuild.wasm npm/esbuild-wasm/wasm_exec.js npm/esbuild-wasm/exit0.js
	rm -r npm/esbuild/install.js
	rm -rf npm/esbuild/lib
	rm -rf npm/esbuild-wasm/esm
	rm -rf npm/esbuild-wasm/lib
	rm -rf require/*/bench/
	rm -rf require/*/demo/
	rm -rf require/*/node_modules/
	go clean -testcache ./internal/...

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

require/spack/node_modules:
	cd require/spack && npm ci

lib/node_modules:
	cd lib && npm ci

scripts/node_modules:
	cd scripts && npm ci

scripts/browser/node_modules:
	cd scripts/browser && npm ci

# This configuration appears to be the equivalent of esbuild's "--minify" and
# "--sourcemap=external" options. However, there's a bug where spack doesn't
# minify top-level variables: https://github.com/swc-project/swc/issues/2451.
SPACK_COMMON_CONFIG += mode: "production",
SPACK_COMMON_CONFIG += options: {
SPACK_COMMON_CONFIG +=   jsc: {
SPACK_COMMON_CONFIG +=     target: "es2021",
SPACK_COMMON_CONFIG +=     minify: {
SPACK_COMMON_CONFIG +=       inlineSourcesContent: true,
SPACK_COMMON_CONFIG +=       sourceMap: true,
SPACK_COMMON_CONFIG +=       compress: true,
SPACK_COMMON_CONFIG +=       mangle: {
SPACK_COMMON_CONFIG +=         topLevel: true,
SPACK_COMMON_CONFIG +=       },
SPACK_COMMON_CONFIG +=     },
SPACK_COMMON_CONFIG +=   },
SPACK_COMMON_CONFIG +=   inlineSourcesContent: true,
SPACK_COMMON_CONFIG +=   sourceMaps: true,
SPACK_COMMON_CONFIG +=   minify: true,
SPACK_COMMON_CONFIG += },

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
	cp -r github/test262/test demo/test262/test

test262: esbuild | demo/test262
	node scripts/test262.js

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

demo-three: demo-three-esbuild demo-three-spack demo-three-rollup demo-three-webpack5 demo-three-parcel2

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

THREE_ROLLUP_CONFIG += import { terser } from 'rollup-plugin-terser';
THREE_ROLLUP_CONFIG += export default {
THREE_ROLLUP_CONFIG +=   output: { format: 'iife', name: 'THREE', sourcemap: true },
THREE_ROLLUP_CONFIG +=   plugins: [terser()],
THREE_ROLLUP_CONFIG += }

demo-three-spack: | require/spack/node_modules demo/three
	rm -fr require/spack/demo/three demo/three/spack
	mkdir -p require/spack/demo/three demo/three/spack
	echo 'import * as THREE from "./src/Three.js"; window.THREE = THREE' > require/spack/demo/three/Three.spack.js

	# Generate the config file
	echo 'module.exports = {' > require/spack/demo/three/spack.config.js
	echo '$(SPACK_COMMON_CONFIG)' >> require/spack/demo/three/spack.config.js
	echo '  entry: { web: "Three.spack.js" },' >> require/spack/demo/three/spack.config.js
	echo '  output: { path: "out", name: "Three.spack.js" },' >> require/spack/demo/three/spack.config.js
	echo '}' >> require/spack/demo/three/spack.config.js

	ln -s ../../../../demo/three/src require/spack/demo/three/src
	ln -s ../../../../demo/three/spack require/spack/demo/three/out
	cd require/spack/demo/three && time -p ../../node_modules/.bin/spack

	# Spack currently requires you to append the sourceMappingURL comment yourself
	echo '//# sourceMappingURL=Three.spack.js.map' >> demo/three/spack/Three.spack.js

	du -h demo/three/spack/Three.spack.js*

demo-three-rollup: | require/rollup/node_modules demo/three
	rm -fr require/rollup/demo/three demo/three/rollup
	mkdir -p require/rollup/demo/three demo/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > require/rollup/demo/three/config.js
	ln -s ../../../../demo/three/src require/rollup/demo/three/src
	ln -s ../../../../demo/three/rollup require/rollup/demo/three/out
	cd require/rollup/demo/three && time -p ../../node_modules/.bin/rollup src/Three.js -o out/Three.rollup.js -c config.js
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

bench-three: bench-three-esbuild bench-three-spack bench-three-rollup bench-three-webpack5 bench-three-parcel2

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

bench-three-spack: | require/spack/node_modules bench/three
	rm -fr require/spack/bench/three bench/three/spack
	mkdir -p require/spack/bench/three bench/three/spack
	echo 'import * as THREE from "./src/entry.js"; window.THREE = THREE' > require/spack/bench/three/entry.spack.js

	# Generate the config file
	echo 'module.exports = {' > require/spack/bench/three/spack.config.js
	echo '$(SPACK_COMMON_CONFIG)' >> require/spack/bench/three/spack.config.js
	echo '  entry: { web: "entry.spack.js" },' >> require/spack/bench/three/spack.config.js
	echo '  output: { path: "out", name: "entry.spack.js" },' >> require/spack/bench/three/spack.config.js
	echo '}' >> require/spack/bench/three/spack.config.js

	ln -s ../../../../bench/three/src require/spack/bench/three/src
	ln -s ../../../../bench/three/spack require/spack/bench/three/out
	cd require/spack/bench/three && time -p ../../node_modules/.bin/spack

	# Spack currently requires you to append the sourceMappingURL comment yourself
	echo '//# sourceMappingURL=entry.spack.js.map' >> bench/three/spack/entry.spack.js

	du -h bench/three/spack/entry.spack.js*

bench-three-rollup: | require/rollup/node_modules bench/three
	rm -fr require/rollup/bench/three bench/three/rollup
	mkdir -p require/rollup/bench/three bench/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > require/rollup/bench/three/config.js
	ln -s ../../../../bench/three/src require/rollup/bench/three/src
	ln -s ../../../../bench/three/rollup require/rollup/bench/three/out
	cd require/rollup/bench/three && time -p ../../node_modules/.bin/rollup src/entry.js -o out/entry.rollup.js -c config.js
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

# This benchmark doesn't currently work because the result crashes with this
# error: "SyntaxError: Identifier 'descriptions' has already been declared"
bench-rome-spack: | require/spack/node_modules bench/rome bench/rome-verify
	rm -fr require/spack/bench/rome bench/rome/spack
	mkdir -p require/spack/bench/rome bench/rome/spack

	# Generate the config file
	echo 'module.exports = {' > require/spack/bench/rome/spack.config.js
	echo '$(SPACK_COMMON_CONFIG)' >> require/spack/bench/rome/spack.config.js
	echo '  target: "node",' >> require/spack/bench/rome/spack.config.js
	echo '  entry: { web: "src/entry.ts" },' >> require/spack/bench/rome/spack.config.js
	echo '  output: { path: "out", name: "rome.spack.js" },' >> require/spack/bench/rome/spack.config.js
	echo '}' >> require/spack/bench/rome/spack.config.js

	# Hack around bugs with support for "paths" and "baseUrl" in "tsconfig.json".
	# See this for more information: https://github.com/swc-project/swc/issues/2725
	echo 'module.exports.options.jsc.paths = {' >> require/spack/bench/rome/spack.config.js
	ls bench/rome/src/@romejs | sed 's/.*/"\@romejs\/&": [__dirname + "\/src\/@romejs\/&\/index.ts"],/g' >> require/spack/bench/rome/spack.config.js
	ls bench/rome/src/@romejs | sed 's/.*/"\@romejs\/&\/*": [__dirname + "\/src\/@romejs\/&\/*.ts"],/g' >> require/spack/bench/rome/spack.config.js
	echo '  "rome": [__dirname + "/src/rome/index.ts"],' >> require/spack/bench/rome/spack.config.js
	echo '  "rome/*": [__dirname + "/src/rome/*.ts"],' >> require/spack/bench/rome/spack.config.js
	echo '}' >> require/spack/bench/rome/spack.config.js

	cp -r bench/rome/src require/spack/bench/rome/src
	ln -s ../../../../bench/rome/spack require/spack/bench/rome/out
	cd require/spack/bench/rome && time -p ../../node_modules/.bin/spack

	# Spack currently requires you to append the sourceMappingURL comment yourself
	echo '//# sourceMappingURL=rome.spack.js.map' >> bench/rome/spack/rome.spack.js

	du -h bench/rome/spack/rome.spack.js*
	cd bench/rome-verify && rm -fr spack && ROME_CACHE=0 node ../rome/spack/rome.spack.js bundle packages/rome spack

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

READMIN_HTML = <meta charset=utf8><div id=root></div><script src=main.js></script>

github/react-admin:
	mkdir -p github
	git clone --depth 1 --branch v3.8.1 https://github.com/marmelab/react-admin.git github/react-admin

bench/readmin: | github/react-admin
	mkdir -p bench/readmin
	cp -r github/react-admin/examples/simple bench/readmin/repo
	cp scripts/readmin-package-lock.json bench/readmin/repo/package-lock.json # Pin package versions for determinism
	cd bench/readmin/repo && npm ci

bench-readmin: bench-readmin-esbuild

READMIN_ESBUILD_FLAGS += --bundle
READMIN_ESBUILD_FLAGS += --define:global=window
READMIN_ESBUILD_FLAGS += --loader:.js=jsx
READMIN_ESBUILD_FLAGS += --minify
READMIN_ESBUILD_FLAGS += --sourcemap
READMIN_ESBUILD_FLAGS += --timing

bench-readmin-esbuild: esbuild | bench/readmin
	rm -fr bench/readmin/esbuild
	time -p ./esbuild $(READMIN_ESBUILD_FLAGS) --outfile=bench/readmin/esbuild/main.js bench/readmin/repo/src/index.js
	echo "$(READMIN_HTML)" > bench/readmin/esbuild/index.html
	du -h bench/readmin/esbuild/main.js*
	shasum bench/readmin/esbuild/main.js*

bench-readmin-eswasm: platform-wasm | bench/readmin
	rm -fr bench/readmin/eswasm
	time -p ./npm/esbuild-wasm/bin/esbuild $(READMIN_ESBUILD_FLAGS) --outfile=bench/readmin/eswasm/main.js bench/readmin/repo/src/index.js
	echo "$(READMIN_HTML)" > bench/readmin/eswasm/index.html
	du -h bench/readmin/eswasm/main.js*
	shasum bench/readmin/eswasm/main.js*
