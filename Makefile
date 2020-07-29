ESBUILD_VERSION = $(shell cat version.txt)

esbuild: cmd/esbuild/*.go pkg/*/*.go internal/*/*.go
	go build "-ldflags=-s -w" ./cmd/esbuild

# These tests are for development
test:
	make -j5 test-go vet-go verify-source-map end-to-end-tests js-api-tests

# These tests are for release ("test-wasm" is not included in "test" because it's pretty slow)
test-all:
	make -j6 test-go vet-go verify-source-map end-to-end-tests js-api-tests test-wasm

# This includes tests of some 3rd-party libraries, which can be very slow
test-extra: test-all test-sucrase test-esprima test-rollup

test-go:
	go test ./internal/...

vet-go:
	go vet ./cmd/... ./internal/... ./pkg/...

test-wasm:
	PATH="$(shell go env GOROOT)/misc/wasm:$$PATH" GOOS=js GOARCH=wasm go test ./internal/...

verify-source-map: | scripts/node_modules
	node scripts/verify-source-map.js

end-to-end-tests: | scripts/node_modules
	node scripts/end-to-end-tests.js

js-api-tests: | scripts/node_modules
	node scripts/js-api-tests.js

update-version-go:
	echo "package main\n\nconst esbuildVersion = \"$(ESBUILD_VERSION)\"" > cmd/esbuild/version.go

platform-all: update-version-go test-all
	make -j11 \
		platform-windows \
		platform-windows-32 \
		platform-darwin \
		platform-freebsd \
		platform-freebsd-arm64 \
		platform-linux \
		platform-linux-32 \
		platform-linux-arm64 \
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

platform-linux-arm64:
	make GOOS=linux GOARCH=arm64 NPMDIR=npm/esbuild-linux-arm64 platform-unixlike

platform-linux-ppc64le:
	make GOOS=linux GOARCH=ppc64le NPMDIR=npm/esbuild-linux-ppc64le platform-unixlike

platform-wasm: | esbuild
	GOOS=js GOARCH=wasm go build -o npm/esbuild-wasm/esbuild.wasm ./cmd/esbuild
	cd npm/esbuild-wasm && npm version "$(ESBUILD_VERSION)" --allow-same-version
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" npm/esbuild-wasm/wasm_exec.js
	mkdir -p npm/esbuild-wasm/lib
	node scripts/esbuild.js ./esbuild --wasm

platform-neutral: | esbuild
	cd npm/esbuild && npm version "$(ESBUILD_VERSION)" --allow-same-version
	node scripts/esbuild.js ./esbuild

publish-all: update-version-go test-all
	make -j10 \
		publish-windows \
		publish-windows-32 \
		publish-darwin \
		publish-freebsd \
		publish-freebsd-arm64 \
		publish-linux \
		publish-linux-32 \
		publish-linux-arm64 \
		publish-linux-ppc64le \
		publish-wasm
	make publish-neutral # Do this after to avoid race conditions
	git commit -am "publish $(ESBUILD_VERSION) to npm"
	git tag "v$(ESBUILD_VERSION)"
	git push origin master "v$(ESBUILD_VERSION)"

publish-windows: platform-windows
	test -n "$(OTP)" && cd npm/esbuild-windows-64 && npm publish --otp="$(OTP)"

publish-windows-32: platform-windows-32
	test -n "$(OTP)" && cd npm/esbuild-windows-32 && npm publish --otp="$(OTP)"

publish-darwin: platform-darwin
	test -n "$(OTP)" && cd npm/esbuild-darwin-64 && npm publish --otp="$(OTP)"

publish-freebsd: platform-freebsd
	test -n "$(OTP)" && cd npm/esbuild-freebsd-64 && npm publish --otp="$(OTP)"

publish-freebsd-arm64: platform-freebsd-arm64
	test -n "$(OTP)" && cd npm/esbuild-freebsd-arm64 && npm publish --otp="$(OTP)"

publish-linux: platform-linux
	test -n "$(OTP)" && cd npm/esbuild-linux-64 && npm publish --otp="$(OTP)"

publish-linux-32: platform-linux-32
	test -n "$(OTP)" && cd npm/esbuild-linux-32 && npm publish --otp="$(OTP)"

publish-linux-arm64: platform-linux-arm64
	test -n "$(OTP)" && cd npm/esbuild-linux-arm64 && npm publish --otp="$(OTP)"

publish-linux-ppc64le: platform-linux-ppc64le
	test -n "$(OTP)" && cd npm/esbuild-linux-ppc64le && npm publish --otp="$(OTP)"

publish-wasm: platform-wasm
	test -n "$(OTP)" && cd npm/esbuild-wasm && npm publish --otp="$(OTP)"

publish-neutral: platform-neutral
	cd npm/esbuild && npm publish

clean:
	rm -f esbuild
	rm -f npm/esbuild-windows-32/esbuild.exe
	rm -f npm/esbuild-windows-64/esbuild.exe
	rm -rf npm/esbuild-darwin-64/bin
	rm -rf npm/esbuild-freebsd-64/bin
	rm -rf npm/esbuild-freebsd-amd64/bin
	rm -rf npm/esbuild-linux-32/bin
	rm -rf npm/esbuild-linux-64/bin
	rm -rf npm/esbuild-linux-arm64/bin
	rm -rf npm/esbuild-linux-ppc64le/bin
	rm -f npm/esbuild-wasm/esbuild.wasm npm/esbuild-wasm/wasm_exec.js
	rm -rf npm/esbuild/lib
	rm -rf npm/esbuild-wasm/lib
	go clean -testcache ./internal/...

node_modules:
	npm ci

# This fixes TypeScript parsing bugs in Parcel 2. Parcel 2 switched to using
# Babel to transform TypeScript into JavaScript, and Babel's TypeScript parser is
# incomplete. It cannot parse the code in the TypeScript benchmark.
#
# The suggested workaround for any Babel bugs is to install a plugin to get the
# old TypeScript parser back. Read this thread for more information:
# https://github.com/parcel-bundler/parcel/issues/2023
PARCELRC += {
PARCELRC +=   "extends": ["@parcel/config-default"],
PARCELRC +=   "transformers": {
PARCELRC +=     "*.ts": ["@parcel/transformer-typescript-tsc"]
PARCELRC +=   }
PARCELRC += }

# There are benchmarks below for both Parcel 1 and Parcel 2. But npm doesn't
# support parallel installs with different versions, so use another directory.
#
# This also uses a symlink as a workaround for a Parcel 2 issue where it
# searches for the "@babel/core" package relative to the input file instead of
# within the "node_modules" folder where Parcel 2 is installed, which causes
# builds to fail.
parcel2/node_modules:
	mkdir parcel2
	echo '{}' > parcel2/package.json
	echo '$(PARCELRC)' > parcel2/.parcelrc
	cd parcel2 && npm install parcel@2.0.0-beta.1 @parcel/transformer-typescript-tsc@2.0.0-beta.1 typescript@3.9.5
	ln -s ../demo parcel2/demo
	ln -s ../bench parcel2/bench

scripts/node_modules:
	cd scripts && npm ci

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
	cd github/uglify && git fetch --depth 1 origin 7a033bb825975a6a729813b2cbe5a722a9047456 && git checkout FETCH_HEAD

demo/uglify: | github/uglify
	mkdir -p demo/uglify
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
	mkdir -p demo/rollup
	cp -RP github/rollup/ demo/rollup
	cd demo/rollup && npm ci

	# Patch over Rollup's custom "package.json" alias using "tsconfig.json"
	cat demo/rollup/tsconfig.json | sed 's/$(TEST_ROLLUP_FIND)/$(TEST_ROLLUP_REPLACE)/' > demo/rollup/tsconfig2.json
	mv demo/rollup/tsconfig2.json demo/rollup/tsconfig.json

test-rollup: esbuild | demo/rollup
	cd demo/rollup && ../../esbuild $(TEST_ROLLUP_FLAGS) && npm run test:only
	cd demo/rollup && ../../esbuild $(TEST_ROLLUP_FLAGS) --minify && npm run test:only

################################################################################
# This builds Sucrase using esbuild and then uses it to run Sucrase's test suite

github/sucrase:
	mkdir -p github/sucrase
	cd github/sucrase && git init && git remote add origin https://github.com/alangpierce/sucrase.git
	cd github/sucrase && git fetch --depth 1 origin a4a596e5cdd57362f309ae50cc32a235d7817d34 && git checkout FETCH_HEAD

demo/sucrase: | github/sucrase
	mkdir -p demo/sucrase
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
	mkdir -p demo/esprima
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
	mkdir -p demo/terser
	cp -r github/terser/ demo/terser
	cd demo/terser && npm ci && npm run build

terser: esbuild | demo/terser
	node scripts/terser-tests.js

################################################################################
# This generates a project containing 10 copies of the Three.js library

github/three:
	mkdir -p github
	git clone --depth 1 --branch r108 https://github.com/mrdoob/three.js.git github/three

demo/three: | github/three
	mkdir -p demo/three
	cp -r github/three/src demo/three/src

bench/three: | github/three
	mkdir -p bench/three
	echo > bench/three/entry.js
	for i in 1 2 3 4 5 6 7 8 9 10; do test -d "bench/three/copy$$i" || cp -r github/three/src "bench/three/copy$$i"; done
	for i in 1 2 3 4 5 6 7 8 9 10; do echo "import * as copy$$i from './copy$$i/Three.js'; export {copy$$i}" >> bench/three/entry.js; done
	echo 'Line count:' && find bench/three -name '*.js' | xargs wc -l | tail -n 1

################################################################################

THREE_ROLLUP_CONFIG += import { terser } from 'rollup-plugin-terser';
THREE_ROLLUP_CONFIG += export default {
THREE_ROLLUP_CONFIG +=   output: { format: 'iife', name: 'THREE', sourcemap: true },
THREE_ROLLUP_CONFIG +=   plugins: [terser()],
THREE_ROLLUP_CONFIG += }

THREE_WEBPACK_FLAGS += --devtool=sourcemap
THREE_WEBPACK_FLAGS += --mode=production
THREE_WEBPACK_FLAGS += --output-library THREE

THREE_PARCEL_FLAGS += --global THREE
THREE_PARCEL_FLAGS += --no-autoinstall
THREE_PARCEL_FLAGS += --out-dir .
THREE_PARCEL_FLAGS += --public-url ./

THREE_FUSEBOX_RUN += require('fuse-box').fusebox({
THREE_FUSEBOX_RUN +=   target: 'browser',
THREE_FUSEBOX_RUN +=   entry: './fusebox-entry.js',
THREE_FUSEBOX_RUN +=   useSingleBundle: true,
THREE_FUSEBOX_RUN +=   output: './dist',
THREE_FUSEBOX_RUN += }).runProd({
THREE_FUSEBOX_RUN +=   bundles: { app: './app.js' },
THREE_FUSEBOX_RUN += });

demo-three: demo-three-esbuild demo-three-rollup demo-three-webpack demo-three-parcel demo-three-fusebox

demo-three-esbuild: esbuild | demo/three
	rm -fr demo/three/esbuild
	mkdir -p demo/three/esbuild
	cd demo/three/esbuild && time -p ../../../esbuild --bundle --global-name=THREE --sourcemap --minify ../src/Three.js --outfile=Three.esbuild.js
	du -h demo/three/esbuild/Three.esbuild.js*
	shasum demo/three/esbuild/Three.esbuild.js*

demo-three-rollup: | node_modules demo/three
	rm -fr demo/three/rollup
	mkdir -p demo/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > demo/three/rollup/config.js
	cd demo/three/rollup && time -p ../../../node_modules/.bin/rollup ../src/Three.js -o Three.rollup.js -c config.js
	du -h demo/three/rollup/Three.rollup.js*

demo-three-webpack: | node_modules demo/three
	rm -fr demo/three/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p demo/three/webpack
	cd demo/three/webpack && time -p ../../../node_modules/.bin/webpack ../src/Three.js $(THREE_WEBPACK_FLAGS) -o Three.webpack.js
	du -h demo/three/webpack/Three.webpack.js*

demo-three-parcel: | node_modules demo/three
	rm -fr demo/three/parcel
	mkdir -p demo/three/parcel
	cd demo/three/parcel && time -p ../../../node_modules/.bin/parcel build ../src/Three.js $(THREE_PARCEL_FLAGS) --out-file Three.parcel.js
	du -h demo/three/parcel/Three.parcel.js*

demo-three-parcel2: | parcel2/node_modules demo/three
	rm -fr demo/three/parcel2
	mkdir -p demo/three/parcel2
	echo 'import * as THREE from "../src/Three.js"; window.THREE = THREE' > demo/three/parcel2/Three.parcel2.js
	cd parcel2 && time -p ./node_modules/.bin/parcel build --no-autoinstall demo/three/src/Three.js \
		--dist-dir ./demo/three/parcel2 --cache-dir ./demo/three/parcel2/.cache
	du -h demo/three/parcel2/Three.parcel2.js*

demo-three-fusebox: | node_modules demo/three
	rm -fr demo/three/fusebox
	mkdir -p demo/three/fusebox
	echo "$(THREE_FUSEBOX_RUN)" > demo/three/fusebox/run.js
	echo 'import * as THREE from "../src/Three.js"; window.THREE = THREE' > demo/three/fusebox/fusebox-entry.js
	cd demo/three/fusebox && time -p node run.js
	du -h demo/three/fusebox/dist/app.js*

################################################################################

bench-three: bench-three-esbuild bench-three-rollup bench-three-webpack bench-three-parcel bench-three-fusebox

bench-three-esbuild: esbuild | bench/three
	rm -fr bench/three/esbuild
	mkdir -p bench/three/esbuild
	cd bench/three/esbuild && time -p ../../../esbuild --bundle --global-name=THREE --sourcemap --minify ../entry.js --outfile=entry.esbuild.js
	du -h bench/three/esbuild/entry.esbuild.js*
	shasum bench/three/esbuild/entry.esbuild.js*

bench-three-rollup: | node_modules bench/three
	rm -fr bench/three/rollup
	mkdir -p bench/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > bench/three/rollup/config.js
	cd bench/three/rollup && time -p ../../../node_modules/.bin/rollup ../entry.js -o entry.rollup.js -c config.js
	du -h bench/three/rollup/entry.rollup.js*

bench-three-webpack: | node_modules bench/three
	rm -fr bench/three/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p bench/three/webpack
	cd bench/three/webpack && time -p ../../../node_modules/.bin/webpack ../entry.js $(THREE_WEBPACK_FLAGS) -o entry.webpack.js
	du -h bench/three/webpack/entry.webpack.js*

bench-three-parcel: | node_modules bench/three
	rm -fr bench/three/parcel
	mkdir -p bench/three/parcel
	cd bench/three/parcel && time -p ../../../node_modules/.bin/parcel build ../entry.js $(THREE_PARCEL_FLAGS) --out-file entry.parcel.js
	du -h bench/three/parcel/entry.parcel.js*

# Note: This is currently broken because it runs out of memory. It's unclear
# how to fix this because the process that runs out of memory is a child worker
# process, and there's no option to pass a "--max-old-space-size" flag to it.
bench-three-parcel2: | parcel2/node_modules bench/three
	rm -fr bench/three/parcel2
	mkdir -p bench/three/parcel2
	echo 'import * as THREE from "../entry.js"; window.THREE = THREE' > bench/three/parcel2/entry.parcel2.js
	cd parcel2 && time -p ./node_modules/.bin/parcel build --no-autoinstall bench/three/parcel2/entry.parcel2.js \
		--dist-dir ./bench/three/parcel2 --cache-dir ./bench/three/parcel2/.cache
	du -h bench/three/parcel2/entry.parcel2.js*

bench-three-fusebox: | node_modules bench/three
	rm -fr bench/three/fusebox
	mkdir -p bench/three/fusebox
	echo "$(THREE_FUSEBOX_RUN)" > bench/three/fusebox/run.js
	echo 'import * as THREE from "../entry.js"; window.THREE = THREE' > bench/three/fusebox/fusebox-entry.js
	cd bench/three/fusebox && time -p node --max-old-space-size=8192 run.js
	du -h bench/three/fusebox/dist/app.js*

################################################################################

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

ROME_WEBPACK_CONFIG += module.exports = {
ROME_WEBPACK_CONFIG +=   entry: '../src/entry.ts',
ROME_WEBPACK_CONFIG +=   mode: 'production',
ROME_WEBPACK_CONFIG +=   target: 'node',
ROME_WEBPACK_CONFIG +=   devtool: 'sourcemap',
ROME_WEBPACK_CONFIG +=   module: { rules: [{ test: /\.ts$$/, loader: 'ts-loader', options: { transpileOnly: true } }] },
ROME_WEBPACK_CONFIG +=   resolve: {
ROME_WEBPACK_CONFIG +=     extensions: ['.ts', '.js'],
ROME_WEBPACK_CONFIG +=     alias: { rome: __dirname + '/../src/rome', '@romejs': __dirname + '/../src/@romejs' },
ROME_WEBPACK_CONFIG +=   },
ROME_WEBPACK_CONFIG +=   output: { filename: 'rome.webpack.js', path: __dirname },
ROME_WEBPACK_CONFIG += };

ROME_PARCEL_FLAGS += --bundle-node-modules
ROME_PARCEL_FLAGS += --no-autoinstall
ROME_PARCEL_FLAGS += --out-dir .
ROME_PARCEL_FLAGS += --public-url ./
ROME_PARCEL_FLAGS += --target node

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

	# Fix a bug where parcel doesn't know about one specific node builtin module
	mkdir -p bench/rome/src/node_modules/inspector
	touch bench/rome/src/node_modules/inspector/index.js

	# These aliases are required to fix parcel path resolution
	echo '{ "alias": {' > bench/rome/src/package.json
	ls bench/rome/src/@romejs | sed 's/.*/"\@romejs\/&": ".\/@romejs\/&",/g' >> bench/rome/src/package.json
	echo '"rome": "./rome" }}' >> bench/rome/src/package.json

	# Get an approximate line count
	rm -r bench/rome/src/@romejs/js-parser/test-fixtures
	echo 'Line count:' && (find bench/rome/src -name '*.ts' && find bench/rome/src -name '*.js') | xargs wc -l | tail -n 1

################################################################################

bench-rome: bench-rome-esbuild bench-rome-webpack bench-rome-parcel

bench-rome-esbuild: esbuild | bench/rome
	rm -fr bench/rome/esbuild
	mkdir -p bench/rome/esbuild
	cd bench/rome/esbuild && time -p ../../../esbuild --bundle --sourcemap --minify ../src/entry.ts --outfile=rome.esbuild.js --platform=node
	du -h bench/rome/esbuild/rome.esbuild.js*
	shasum bench/rome/esbuild/rome.esbuild.js*

bench-rome-webpack: | node_modules bench/rome
	rm -fr bench/rome/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p bench/rome/webpack
	echo "$(ROME_WEBPACK_CONFIG)" > bench/rome/webpack/webpack.config.js
	cd bench/rome/webpack && time -p ../../../node_modules/.bin/webpack
	du -h bench/rome/webpack/rome.webpack.js*

bench-rome-parcel: | node_modules bench/rome
	rm -fr bench/rome/parcel
	mkdir -p bench/rome/parcel
	cd bench/rome/parcel && time -p ../../../node_modules/.bin/parcel build ../src/entry.ts $(ROME_PARCEL_FLAGS) --out-file rome.parcel.js
	du -h bench/rome/parcel/rome.parcel.js*

# Note: This is currently broken because Parcel 2 can't handle TypeScript files
# that re-export types.
bench-rome-parcel2: | parcel2/node_modules bench/rome
	rm -fr bench/rome/parcel2
	mkdir -p bench/rome/parcel2
	cd parcel2 && time -p ./node_modules/.bin/parcel build --no-autoinstall bench/rome/src/entry.ts \
		--dist-dir ./bench/rome/parcel2 --cache-dir ./bench/rome/parcel2/.cache
	du -h bench/rome/parcel2/rome.parcel2.js*
