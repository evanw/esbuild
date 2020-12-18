ESBUILD_VERSION = $(shell cat version.txt)
JOBS = 6

esbuild: cmd/esbuild/version.go cmd/esbuild/*.go pkg/*/*.go internal/*/*.go go.mod
	go build "-ldflags=-s -w" ./cmd/esbuild

npm/esbuild-wasm/esbuild.wasm: cmd/esbuild/version.go cmd/esbuild/*.go pkg/*/*.go internal/*/*.go
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" npm/esbuild-wasm/wasm_exec.js
	GOOS=js GOARCH=wasm go build -o npm/esbuild-wasm/esbuild.wasm ./cmd/esbuild

test:
	make -j$(JOBS) test-common

# These tests are for development
test-common: test-go vet-go verify-source-map end-to-end-tests js-api-tests plugin-tests

# These tests are for release (the extra tests are not included in "test" because they are pretty slow)
test-all:
	make -j$(JOBS) test-common ts-type-tests test-wasm-node test-wasm-browser

# This includes tests of some 3rd-party libraries, which can be very slow
test-prepublish: check-go-version test-all test-preact-splitting test-sucrase bench-rome-esbuild test-esprima test-rollup

check-go-version:
	@go version | grep 'go1\.15\.5' || (echo 'Please install Go version 1.15.5' && false)

test-go:
	go test ./internal/...

vet-go:
	go vet ./cmd/... ./internal/... ./pkg/...

fmt-go:
	go fmt ./cmd/... ./internal/... ./pkg/...

test-wasm-node: platform-wasm
	PATH="$(shell go env GOROOT)/misc/wasm:$$PATH" GOOS=js GOARCH=wasm go test ./internal/...
	npm/esbuild-wasm/bin/esbuild --version

test-wasm-browser: platform-wasm | scripts/browser/node_modules
	cd scripts/browser && node browser-tests.js

register-test: cmd/esbuild/version.go | scripts/node_modules
	cd npm/esbuild && npm version "$(ESBUILD_VERSION)" --allow-same-version
	node scripts/register-test.js

verify-source-map: cmd/esbuild/version.go | scripts/node_modules
	cd npm/esbuild && npm version "$(ESBUILD_VERSION)" --allow-same-version
	node scripts/verify-source-map.js

end-to-end-tests: cmd/esbuild/version.go | scripts/node_modules
	cd npm/esbuild && npm version "$(ESBUILD_VERSION)" --allow-same-version
	node scripts/end-to-end-tests.js

js-api-tests: cmd/esbuild/version.go | scripts/node_modules
	cd npm/esbuild && npm version "$(ESBUILD_VERSION)" --allow-same-version
	node scripts/js-api-tests.js

plugin-tests: cmd/esbuild/version.go | scripts/node_modules
	node scripts/plugin-tests.js

ts-type-tests: | scripts/node_modules
	node scripts/ts-type-tests.js

lib-typecheck: | lib/node_modules
	cd lib && node_modules/.bin/tsc -noEmit -p .

cmd/esbuild/version.go: version.txt
	node -e 'console.log(`package main\n\nconst esbuildVersion = "$(ESBUILD_VERSION)"`)' > cmd/esbuild/version.go

platform-all: cmd/esbuild/version.go test-all
	make -j8 \
		platform-windows \
		platform-windows-32 \
		platform-darwin \
		platform-freebsd \
		platform-freebsd-arm64 \
		platform-linux \
		platform-linux-32 \
		platform-linux-arm \
		platform-linux-arm64 \
		platform-linux-mips64le \
		platform-linux-ppc64le \
		platform-wasm \
		platform-neutral

platform-windows:
	cd npm/esbuild-windows-64 && npm version "$(ESBUILD_VERSION)" --allow-same-version
	GOOS=windows GOARCH=amd64 go build "-ldflags=-s -w" -o npm/esbuild-windows-64/esbuild.exe ./cmd/esbuild

platform-windows-32:
	cd npm/esbuild-windows-32 && npm version "$(ESBUILD_VERSION)" --allow-same-version
	GOOS=windows GOARCH=386 go build "-ldflags=-s -w" -o npm/esbuild-windows-32/esbuild.exe ./cmd/esbuild

platform-unixlike:
	test -n "$(GOOS)" && test -n "$(GOARCH)" && test -n "$(NPMDIR)"
	mkdir -p "$(NPMDIR)/bin"
	cd "$(NPMDIR)" && npm version "$(ESBUILD_VERSION)" --allow-same-version
	GOOS="$(GOOS)" GOARCH="$(GOARCH)" go build "-ldflags=-s -w" -o "$(NPMDIR)/bin/esbuild" ./cmd/esbuild

platform-darwin:
	make GOOS=darwin GOARCH=amd64 NPMDIR=npm/esbuild-darwin-64 platform-unixlike

platform-freebsd:
	make GOOS=freebsd GOARCH=amd64 NPMDIR=npm/esbuild-freebsd-64 platform-unixlike

platform-freebsd-arm64:
	make GOOS=freebsd GOARCH=arm64 NPMDIR=npm/esbuild-freebsd-arm64 platform-unixlike

platform-linux:
	make GOOS=linux GOARCH=amd64 NPMDIR=npm/esbuild-linux-64 platform-unixlike

platform-linux-32:
	make GOOS=linux GOARCH=386 NPMDIR=npm/esbuild-linux-32 platform-unixlike

platform-linux-arm:
	make GOOS=linux GOARCH=arm NPMDIR=npm/esbuild-linux-arm platform-unixlike

platform-linux-arm64:
	make GOOS=linux GOARCH=arm64 NPMDIR=npm/esbuild-linux-arm64 platform-unixlike

platform-linux-mips64le:
	make GOOS=linux GOARCH=mips64le NPMDIR=npm/esbuild-linux-mips64le platform-unixlike

platform-linux-ppc64le:
	make GOOS=linux GOARCH=ppc64le NPMDIR=npm/esbuild-linux-ppc64le platform-unixlike

platform-wasm: esbuild npm/esbuild-wasm/esbuild.wasm | scripts/node_modules
	cd npm/esbuild-wasm && npm version "$(ESBUILD_VERSION)" --allow-same-version
	mkdir -p npm/esbuild-wasm/lib
	node scripts/esbuild.js ./esbuild --wasm

platform-neutral: esbuild lib-typecheck | scripts/node_modules
	cd npm/esbuild && npm version "$(ESBUILD_VERSION)" --allow-same-version
	node scripts/esbuild.js ./esbuild

test-otp:
	test -n "$(OTP)" && echo publish --otp="$(OTP)"

publish-all: cmd/esbuild/version.go test-prepublish
	rm -fr npm && git checkout npm
	@make -j4 \
		publish-windows \
		publish-windows-32 \
		publish-freebsd \
		publish-freebsd-arm64
	@make -j4 \
		publish-darwin \
		publish-linux \
		publish-linux-32
	@make -j4 \
		publish-linux-arm \
		publish-linux-arm64 \
		publish-linux-mips64le \
		publish-linux-ppc64le
	# Do these last to avoid race conditions
	@make -j2 \
		publish-neutral \
		publish-wasm
	git tag "v$(ESBUILD_VERSION)"
	git push origin "v$(ESBUILD_VERSION)"

publish-windows: platform-windows
	cd npm/esbuild-windows-64 && npm publish --access=public

publish-windows-32: platform-windows-32
	cd npm/esbuild-windows-32 && npm publish --access=public

publish-darwin: platform-darwin
	cd npm/esbuild-darwin-64 && npm publish --access=public

publish-freebsd: platform-freebsd
	cd npm/esbuild-freebsd-64 && npm publish --access=public

publish-freebsd-arm64: platform-freebsd-arm64
	cd npm/esbuild-freebsd-arm64 && npm publish --access=public

publish-linux: platform-linux
	cd npm/esbuild-linux-64 && npm publish --access=public

publish-linux-32: platform-linux-32
	cd npm/esbuild-linux-32 && npm publish --access=public

publish-linux-arm: platform-linux-arm
	cd npm/esbuild-linux-arm && npm publish --access=public

publish-linux-arm64: platform-linux-arm64
	cd npm/esbuild-linux-arm64 && npm publish --access=public

publish-linux-mips64le: platform-linux-mips64le
	cd npm/esbuild-linux-mips64le && npm publish --access=public

publish-linux-ppc64le: platform-linux-ppc64le
	cd npm/esbuild-linux-ppc64le && npm publish --access=public

publish-wasm: platform-wasm
	cd npm/esbuild-wasm && npm publish --access=public

publish-neutral: platform-neutral
	cd npm/esbuild && npm publish --access=public

clean:
	rm -f esbuild
	rm -f npm/esbuild-windows-32/esbuild.exe
	rm -f npm/esbuild-windows-64/esbuild.exe
	rm -rf npm/esbuild-darwin-64/bin
	rm -rf npm/esbuild-freebsd-64/bin
	rm -rf npm/esbuild-freebsd-amd64/bin
	rm -rf npm/esbuild-linux-32/bin
	rm -rf npm/esbuild-linux-64/bin
	rm -rf npm/esbuild-linux-arm/bin
	rm -rf npm/esbuild-linux-arm64/bin
	rm -rf npm/esbuild-linux-mips64le/bin
	rm -rf npm/esbuild-linux-ppc64le/bin
	rm -f npm/esbuild-wasm/esbuild.wasm npm/esbuild-wasm/wasm_exec.js
	rm -rf npm/esbuild/lib
	rm -rf npm/esbuild-wasm/lib
	go clean -testcache ./internal/...

# This also cleans directories containing cached code from other projects
clean-all: clean
	rm -fr github demo bench

################################################################################
# These npm packages are used for benchmarks. Instal them in subdirectories
# because we want to install the same package name at multiple versions

require/webpack/node_modules:
	mkdir -p require/webpack
	echo '{}' > require/webpack/package.json
	cd require/webpack && npm install webpack@4.44.2 webpack-cli@3.3.12 ts-loader@8.0.4 typescript@4.0.3

require/webpack5/node_modules:
	mkdir -p require/webpack5
	echo '{}' > require/webpack5/package.json
	cd require/webpack5 && npm install webpack@5.0.0-rc.4 webpack-cli@4.0.0-rc.1 ts-loader@8.0.4 typescript@4.0.3

require/rollup/node_modules:
	mkdir -p require/rollup
	echo '{}' > require/rollup/package.json
	cd require/rollup && npm install rollup@2.29.0 rollup-plugin-terser@7.0.2

require/parcel/node_modules:
	mkdir -p require/parcel
	echo '{}' > require/parcel/package.json
	cd require/parcel && npm install parcel@1.12.4 typescript@4.1.2

	# Fix a bug where parcel doesn't know about one specific node builtin module
	mkdir -p require/parcel/node_modules/inspector
	touch require/parcel/node_modules/inspector/index.js

require/fusebox/node_modules:
	mkdir -p require/fusebox
	echo '{}' > require/fusebox/package.json
	cd require/fusebox && npm install fuse-box@4.0.0-next.444

require/parcel2/node_modules:
	mkdir -p require/parcel2
	echo '{}' > require/parcel2/package.json
	cd require/parcel2 && npm install parcel@2.0.0-nightly.475 @parcel/transformer-typescript-tsc@2.0.0-nightly.477 typescript@4.1.2

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
	cp -r github/test262/test demo/test262/test

test262: esbuild | demo/test262
	node scripts/test262.js

################################################################################
# This runs UglifyJS's test suite through esbuild

github/uglify:
	mkdir -p github/uglify
	cd github/uglify && git init && git remote add origin https://github.com/mishoo/uglifyjs.git
	cd github/uglify && git fetch --depth 1 origin 83a3cbf1514e81292b749655f2f712e82a5a2ba8 && git checkout FETCH_HEAD

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
	git clone --depth 1 --branch v2.15.0 https://github.com/rollup/rollup.git github/rollup

demo/rollup: | github/rollup
	mkdir -p demo
	cp -RP github/rollup/ demo/rollup
	cd demo/rollup && npm ci

	# Patch over Rollup's custom "package.json" alias using "tsconfig.json"
	cat demo/rollup/tsconfig.json | sed -e 's/$(TEST_ROLLUP_FIND)/$(TEST_ROLLUP_REPLACE)/' > demo/rollup/tsconfig2.json
	mv demo/rollup/tsconfig2.json demo/rollup/tsconfig.json

test-rollup: esbuild | demo/rollup
	cd demo/rollup && ../../esbuild $(TEST_ROLLUP_FLAGS) && npm run test:only
	cd demo/rollup && ../../esbuild $(TEST_ROLLUP_FLAGS) --minify && npm run test:only

################################################################################
# This builds Sucrase using esbuild and then uses it to run Sucrase's test suite

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
	cd demo/sucrase && find test -name '*.ts' | sed -e 's/\(.*\)\.ts/import ".\/\1"/g' > all-tests.ts
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

demo-three: demo-three-esbuild demo-three-rollup demo-three-webpack demo-three-webpack5 demo-three-parcel demo-three-parcel2 demo-three-fusebox

demo-three-esbuild: esbuild | demo/three
	rm -fr demo/three/esbuild
	time -p ./esbuild --bundle --global-name=THREE --sourcemap --minify demo/three/src/Three.js --outfile=demo/three/esbuild/Three.esbuild.js
	du -h demo/three/esbuild/Three.esbuild.js*
	shasum demo/three/esbuild/Three.esbuild.js*

demo-three-eswasm: npm/esbuild-wasm/esbuild.wasm | demo/three
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

demo-three-rollup: | require/rollup/node_modules demo/three
	rm -fr require/rollup/demo/three demo/three/rollup
	mkdir -p require/rollup/demo/three demo/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > require/rollup/demo/three/config.js
	ln -s ../../../../demo/three/src require/rollup/demo/three/src
	ln -s ../../../../demo/three/rollup require/rollup/demo/three/out
	cd require/rollup/demo/three && time -p ../../node_modules/.bin/rollup src/Three.js -o out/Three.rollup.js -c config.js
	du -h demo/three/rollup/Three.rollup.js*

THREE_WEBPACK_FLAGS += --devtool=sourcemap
THREE_WEBPACK_FLAGS += --mode=production
THREE_WEBPACK_FLAGS += --output-library THREE

demo-three-webpack: | require/webpack/node_modules demo/three
	rm -fr require/webpack/demo/three demo/three/webpack require/webpack/node_modules/.cache/terser-webpack-plugin
	mkdir -p require/webpack/demo/three demo/three/webpack
	ln -s ../../../../demo/three/src require/webpack/demo/three/src
	ln -s ../../../../demo/three/webpack require/webpack/demo/three/out
	cd require/webpack/demo/three && time -p ../../node_modules/.bin/webpack src/Three.js $(THREE_WEBPACK_FLAGS) -o out/Three.webpack.js
	du -h demo/three/webpack/Three.webpack.js*

THREE_WEBPACK5_FLAGS += --devtool=source-map
THREE_WEBPACK5_FLAGS += --mode=production
THREE_WEBPACK5_FLAGS += --output-library THREE

demo-three-webpack5: | require/webpack5/node_modules demo/three
	rm -fr require/webpack5/demo/three demo/three/webpack5
	mkdir -p require/webpack5/demo/three demo/three/webpack5
	ln -s ../../../../demo/three/src require/webpack5/demo/three/src
	ln -s ../../../../demo/three/webpack5 require/webpack5/demo/three/out
	cd require/webpack5/demo/three && time -p ../../node_modules/.bin/webpack ./src/Three.js $(THREE_WEBPACK5_FLAGS) -o out/Three.webpack5.js
	du -h demo/three/webpack5/Three.webpack5.js*

THREE_PARCEL_FLAGS += --global THREE
THREE_PARCEL_FLAGS += --no-autoinstall
THREE_PARCEL_FLAGS += --out-dir out
THREE_PARCEL_FLAGS += --public-url ./

demo-three-parcel: | require/parcel/node_modules demo/three
	rm -fr require/parcel/demo/three demo/three/parcel
	mkdir -p require/parcel/demo/three demo/three/parcel
	ln -s ../../../../demo/three/src require/parcel/demo/three/src
	ln -s ../../../../demo/three/parcel require/parcel/demo/three/out
	cd require/parcel/demo/three && time -p ../../node_modules/.bin/parcel build src/Three.js $(THREE_PARCEL_FLAGS) --out-file Three.parcel.js
	du -h demo/three/parcel/Three.parcel.js*

demo-three-parcel2: | require/parcel2/node_modules demo/three
	rm -fr require/parcel2/demo/three demo/three/parcel2
	mkdir -p require/parcel2/demo/three demo/three/parcel2
	ln -s ../../../../demo/three/src require/parcel2/demo/three/src
	echo 'import * as THREE from "./src/Three.js"; window.THREE = THREE' > require/parcel2/demo/three/Three.parcel2.js
	cd require/parcel2/demo/three && time -p ../../node_modules/.bin/parcel build \
		Three.parcel2.js --dist-dir ../../../../demo/three/parcel2 --cache-dir .cache
	du -h demo/three/parcel2/Three.parcel2.js*

THREE_FUSEBOX_RUN += require('fuse-box').fusebox({
THREE_FUSEBOX_RUN +=   target: 'browser',
THREE_FUSEBOX_RUN +=   entry: './fusebox-entry.js',
THREE_FUSEBOX_RUN +=   useSingleBundle: true,
THREE_FUSEBOX_RUN +=   output: './dist',
THREE_FUSEBOX_RUN += }).runProd({
THREE_FUSEBOX_RUN +=   bundles: { app: './app.js' },
THREE_FUSEBOX_RUN += });

demo-three-fusebox: | require/fusebox/node_modules demo/three
	rm -fr require/fusebox/demo/three demo/three/fusebox
	mkdir -p require/fusebox/demo/three demo/three/fusebox
	echo "$(THREE_FUSEBOX_RUN)" > require/fusebox/demo/three/run.js
	ln -s ../../../../demo/three/src require/fusebox/demo/three/src
	ln -s ../../../../demo/three/fusebox require/fusebox/demo/three/dist
	echo 'import * as THREE from "./src/Three.js"; window.THREE = THREE' > require/fusebox/demo/three/fusebox-entry.js
	cd require/fusebox/demo/three && time -p node run.js
	du -h demo/three/fusebox/app.js*

################################################################################
# three.js benchmark (measures JavaScript performance, same as three.js demo but 10x bigger)

bench/three: | github/three
	mkdir -p bench/three/src
	echo > bench/three/src/entry.js
	for i in 1 2 3 4 5 6 7 8 9 10; do test -d "bench/three/src/copy$$i" || cp -r github/three/src "bench/three/src/copy$$i"; done
	for i in 1 2 3 4 5 6 7 8 9 10; do echo "import * as copy$$i from './copy$$i/Three.js'; export {copy$$i}" >> bench/three/src/entry.js; done
	echo 'Line count:' && find bench/three/src -name '*.js' | xargs wc -l | tail -n 1

bench-three: bench-three-esbuild bench-three-rollup bench-three-webpack bench-three-webpack5 bench-three-parcel bench-three-fusebox

bench-three-esbuild: esbuild | bench/three
	rm -fr bench/three/esbuild
	time -p ./esbuild --bundle --global-name=THREE --sourcemap --minify bench/three/src/entry.js --outfile=bench/three/esbuild/entry.esbuild.js
	du -h bench/three/esbuild/entry.esbuild.js*
	shasum bench/three/esbuild/entry.esbuild.js*

bench-three-eswasm: npm/esbuild-wasm/esbuild.wasm | bench/three
	rm -fr bench/three/eswasm
	time -p ./npm/esbuild-wasm/bin/esbuild --bundle --global-name=THREE \
		--sourcemap --minify bench/three/src/entry.js --outfile=bench/three/eswasm/entry.eswasm.js
	du -h bench/three/eswasm/entry.eswasm.js*
	shasum bench/three/eswasm/entry.eswasm.js*

bench-three-rollup: | require/rollup/node_modules bench/three
	rm -fr require/rollup/bench/three bench/three/rollup
	mkdir -p require/rollup/bench/three bench/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > require/rollup/bench/three/config.js
	ln -s ../../../../bench/three/src require/rollup/bench/three/src
	ln -s ../../../../bench/three/rollup require/rollup/bench/three/out
	cd require/rollup/bench/three && time -p ../../node_modules/.bin/rollup src/entry.js -o out/entry.rollup.js -c config.js
	du -h bench/three/rollup/entry.rollup.js*

bench-three-webpack: | require/webpack/node_modules bench/three
	rm -fr require/webpack/bench/three bench/three/webpack require/webpack/node_modules/.cache/terser-webpack-plugin
	mkdir -p require/webpack/bench/three bench/three/webpack
	ln -s ../../../../bench/three/src require/webpack/bench/three/src
	ln -s ../../../../bench/three/webpack require/webpack/bench/three/out
	cd require/webpack/bench/three && time -p ../../node_modules/.bin/webpack src/entry.js $(THREE_WEBPACK_FLAGS) -o out/entry.webpack.js
	du -h bench/three/webpack/entry.webpack.js*

bench-three-webpack5: | require/webpack5/node_modules bench/three
	rm -fr require/webpack5/bench/three bench/three/webpack5
	mkdir -p require/webpack5/bench/three bench/three/webpack5
	ln -s ../../../../bench/three/src require/webpack5/bench/three/src
	ln -s ../../../../bench/three/webpack5 require/webpack5/bench/three/out
	cd require/webpack5/bench/three && time -p ../../node_modules/.bin/webpack ./src/entry.js $(THREE_WEBPACK5_FLAGS) -o out/entry.webpack5.js
	du -h bench/three/webpack5/entry.webpack5.js*

bench-three-parcel: | require/parcel/node_modules bench/three
	rm -fr require/parcel/bench/three bench/three/parcel
	mkdir -p require/parcel/bench/three bench/three/parcel
	ln -s ../../../../bench/three/src require/parcel/bench/three/src
	ln -s ../../../../bench/three/parcel require/parcel/bench/three/out
	cd require/parcel/bench/three && time -p ../../node_modules/.bin/parcel build src/entry.js $(THREE_PARCEL_FLAGS) --out-file entry.parcel.js
	du -h bench/three/parcel/entry.parcel.js*

bench-three-parcel2: | require/parcel2/node_modules bench/three
	rm -fr require/parcel2/bench/three bench/three/parcel2
	mkdir -p require/parcel2/bench/three bench/three/parcel2
	ln -s ../../../../bench/three/src require/parcel2/bench/three/src
	echo 'import * as THREE from "./src/entry.js"; window.THREE = THREE' > require/parcel2/bench/three/entry.parcel2.js
	cd require/parcel2/bench/three && time -p node --max-old-space-size=4096 ../../node_modules/.bin/parcel build \
		entry.parcel2.js --dist-dir ../../../../bench/three/parcel2 --cache-dir .cache
	du -h bench/three/parcel2/entry.parcel2.js*

bench-three-fusebox: | require/fusebox/node_modules bench/three
	rm -fr require/fusebox/bench/three bench/three/fusebox
	mkdir -p require/fusebox/bench/three bench/three/fusebox
	echo "$(THREE_FUSEBOX_RUN)" > require/fusebox/bench/three/run.js
	ln -s ../../../../bench/three/src require/fusebox/bench/three/src
	ln -s ../../../../bench/three/fusebox require/fusebox/bench/three/dist
	echo 'import * as THREE from "./src/entry.js"; window.THREE = THREE' > require/fusebox/bench/three/fusebox-entry.js
	cd require/fusebox/bench/three && time -p node --max-old-space-size=8192 run.js
	du -h bench/three/fusebox/app.js*

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
	sed -e "/createHook/d" bench/rome/src/@romejs/js-compiler/index.ts >> .temp
	mv .temp bench/rome/src/@romejs/js-compiler/index.ts

	# Replace "import fs = require('fs')" with "const fs = require('fs')" because
	# the TypeScript compiler strips these statements when targeting "esnext",
	# which breaks Parcel 2 when scope hoisting is enabled.
	find bench/rome/src -name '*.ts' -type f -print0 | xargs -L1 -0 sed -i'' -e 's/import \([A-Za-z0-9_]*\) =/const \1 =/g'
	find bench/rome/src -name '*.tsx' -type f -print0 | xargs -L1 -0 sed -i'' -e 's/import \([A-Za-z0-9_]*\) =/const \1 =/g'

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

bench-rome: bench-rome-esbuild bench-rome-webpack bench-rome-parcel

bench-rome-esbuild: esbuild | bench/rome bench/rome-verify
	rm -fr bench/rome/esbuild
	time -p ./esbuild --bundle --sourcemap --minify bench/rome/src/entry.ts --outfile=bench/rome/esbuild/rome.esbuild.js --platform=node
	du -h bench/rome/esbuild/rome.esbuild.js*
	shasum bench/rome/esbuild/rome.esbuild.js*
	# cd bench/rome-verify && rm -fr esbuild && ROME_CACHE=0 node ../rome/esbuild/rome.esbuild.js bundle packages/rome esbuild

ROME_WEBPACK_CONFIG += module.exports = {
ROME_WEBPACK_CONFIG +=   entry: './src/entry.ts',
ROME_WEBPACK_CONFIG +=   mode: 'production',
ROME_WEBPACK_CONFIG +=   target: 'node',
ROME_WEBPACK_CONFIG +=   devtool: 'sourcemap',
ROME_WEBPACK_CONFIG +=   module: { rules: [{ test: /\.ts$$/, loader: 'ts-loader', options: { transpileOnly: true } }] },
ROME_WEBPACK_CONFIG +=   resolve: {
ROME_WEBPACK_CONFIG +=     extensions: ['.ts', '.js'],
ROME_WEBPACK_CONFIG +=     alias: { rome: __dirname + '/src/rome', '@romejs': __dirname + '/src/@romejs' },
ROME_WEBPACK_CONFIG +=   },
ROME_WEBPACK_CONFIG +=   output: { filename: 'rome.webpack.js', path: __dirname + '/out' },
ROME_WEBPACK_CONFIG += };

bench-rome-webpack: | require/webpack/node_modules bench/rome bench/rome-verify
	rm -fr require/webpack/bench/rome bench/rome/webpack require/webpack/node_modules/.cache/terser-webpack-plugin
	mkdir -p require/webpack/bench/rome bench/rome/webpack
	echo "$(ROME_WEBPACK_CONFIG)" > require/webpack/bench/rome/webpack.config.js
	ln -s ../../../../bench/rome/src require/webpack/bench/rome/src
	ln -s ../../../../bench/rome/webpack require/webpack/bench/rome/out
	cd require/webpack/bench/rome && time -p ../../node_modules/.bin/webpack
	du -h bench/rome/webpack/rome.webpack.js*
	cd bench/rome-verify && rm -fr webpack && ROME_CACHE=0 node ../rome/webpack/rome.webpack.js bundle packages/rome webpack

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

ROME_PARCEL_FLAGS += --bundle-node-modules
ROME_PARCEL_FLAGS += --no-autoinstall
ROME_PARCEL_FLAGS += --out-dir .
ROME_PARCEL_FLAGS += --public-url ./
ROME_PARCEL_FLAGS += --target node

ROME_PARCEL_ALIASES += "alias": {
ROME_PARCEL_ALIASES +=   $(shell ls bench/rome/src/@romejs | sed -e 's/.*/"\@romejs\/&": ".\/@romejs\/&",/g')
ROME_PARCEL_ALIASES +=   "rome": "./rome"
ROME_PARCEL_ALIASES += }

bench-rome-parcel: | require/parcel/node_modules bench/rome bench/rome-verify
	rm -fr bench/rome/parcel
	cp -r bench/rome/src bench/rome/parcel
	rm -fr bench/rome/parcel/node_modules
	cp -RP require/parcel/node_modules bench/rome/parcel/node_modules

	# Inject aliases into "package.json" to fix Parcel ignoring "tsconfig.json".
	cat require/parcel/package.json | sed -e '/^\}/d' > bench/rome/parcel/package.json
	echo ', $(ROME_PARCEL_ALIASES) }' >> bench/rome/parcel/package.json

	# Work around a bug that causes the resulting bundle to crash when run.
	# See https://github.com/parcel-bundler/parcel/issues/1762 for more info.
	echo 'import "regenerator-runtime/runtime"; import "./entry.ts"' > bench/rome/parcel/rome.parcel.ts

	cd bench/rome/parcel && time -p node_modules/.bin/parcel build rome.parcel.ts $(ROME_PARCEL_FLAGS) --out-file rome.parcel.js
	du -h bench/rome/parcel/rome.parcel.js*
	cd bench/rome-verify && rm -fr parcel && ROME_CACHE=0 node ../rome/parcel/rome.parcel.js bundle packages/rome parcel

# This fixes TypeScript parsing bugs in Parcel 2. Parcel 2 switched to using
# Babel to transform TypeScript into JavaScript, and Babel's TypeScript parser is
# incomplete. It cannot parse the code in the TypeScript benchmark.
#
# The suggested workaround for any Babel bugs is to install a plugin to get the
# old TypeScript parser back. Read this thread for more information:
# https://github.com/parcel-bundler/parcel/issues/2023.
#
# It also looks like the Parcel team is considering reverting this change:
# https://github.com/parcel-bundler/parcel/issues/4938.
PARCELRC += {
PARCELRC +=   "extends": ["@parcel/config-default"],
PARCELRC +=   "transformers": {
PARCELRC +=     "*.ts": ["@parcel/transformer-typescript-tsc"]
PARCELRC +=   }
PARCELRC += }

bench-rome-parcel2: | require/parcel2/node_modules bench/rome bench/rome-verify
	rm -fr bench/rome/parcel2
	cp -r bench/rome/src bench/rome/parcel2 # Can't use a symbolic link or ".parcelrc" breaks
	rm -fr bench/rome/parcel2/node_modules
	cp -RP require/parcel2/node_modules bench/rome/parcel2/node_modules
	echo '$(PARCELRC)' > bench/rome/parcel2/.parcelrc

	# Inject aliases into "package.json" to fix Parcel 2 ignoring "tsconfig.json".
	# Also inject "engines": "node" to avoid Parcel 2 mangling node globals.
	cat require/parcel2/package.json | sed -e '/^\}/d' > bench/rome/parcel2/package.json
	echo ', "engines": { "node": "0.0.0" }' >> bench/rome/parcel2/package.json
	echo ', $(ROME_PARCEL_ALIASES) }' >> bench/rome/parcel2/package.json

	# Work around a bug that causes the resulting bundle to crash when run.
	# See https://github.com/parcel-bundler/parcel/issues/1762 for more info.
	echo 'import "regenerator-runtime/runtime"; import "./entry.ts"' > bench/rome/parcel2/rome.parcel.ts

	cd bench/rome/parcel2 && time -p node_modules/.bin/parcel build rome.parcel.ts --dist-dir . --cache-dir .cache
	du -h bench/rome/parcel2/rome.parcel.js*
	cd bench/rome-verify && rm -fr parcel2 && ROME_CACHE=0 node ../rome/parcel2/rome.parcel.js bundle packages/rome parcel2

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

bench-readmin-esbuild: esbuild | bench/readmin
	rm -fr bench/readmin/esbuild
	time -p ./esbuild --bundle --minify --loader:.js=jsx --define:process.env.NODE_ENV='"production"' \
		--define:global=window --sourcemap --outfile=bench/readmin/esbuild/main.js bench/readmin/repo/src/index.js
	echo "$(READMIN_HTML)" > bench/readmin/esbuild/index.html
	du -h bench/readmin/esbuild/main.js*
	shasum bench/readmin/esbuild/main.js*

bench-readmin-eswasm: npm/esbuild-wasm/esbuild.wasm | bench/readmin
	rm -fr bench/readmin/eswasm
	time -p ./npm/esbuild-wasm/bin/esbuild \
		--bundle --minify --loader:.js=jsx --define:process.env.NODE_ENV='"production"' \
		--define:global=window --sourcemap --outfile=bench/readmin/eswasm/main.js bench/readmin/repo/src/index.js
	echo "$(READMIN_HTML)" > bench/readmin/eswasm/index.html
	du -h bench/readmin/eswasm/main.js*
	shasum bench/readmin/eswasm/main.js*
