esbuild: src/esbuild/*/*.go
	GOPATH=`pwd` go build -o esbuild esbuild/main

test:
	GOPATH=`pwd` go test ./...

platform-all: test
	make -j4 platform-windows platform-darwin platform-linux platform-wasm

platform-windows:
	mkdir -p npm/esbuild-windows-64/bin
	GOOS=windows GOARCH=amd64 GOPATH=`pwd` go build -o npm/esbuild-windows-64/esbuild.exe esbuild/main

platform-darwin:
	mkdir -p npm/esbuild-darwin-64/bin
	GOOS=darwin GOARCH=amd64 GOPATH=`pwd` go build -o npm/esbuild-darwin-64/bin/esbuild esbuild/main

platform-linux:
	mkdir -p npm/esbuild-linux-64/bin
	GOOS=linux GOARCH=amd64 GOPATH=`pwd` go build -o npm/esbuild-linux-64/bin/esbuild esbuild/main

platform-wasm:
	GOOS=js GOARCH=wasm GOPATH=`pwd` go build -o npm/esbuild-wasm/esbuild.wasm esbuild/main
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" npm/esbuild-wasm/wasm_exec.js

publish-all: test
	make -j4 publish-windows publish-darwin publish-linux publish-wasm

publish-windows: platform-windows
	[ ! -z "$(OTP)" ] && cd npm/esbuild-windows-64 && npm publish --otp="$(OTP)"

publish-darwin: platform-darwin
	[ ! -z "$(OTP)" ] && cd npm/esbuild-darwin-64 && npm publish --otp="$(OTP)"

publish-linux: platform-linux
	[ ! -z "$(OTP)" ] && cd npm/esbuild-linux-64 && npm publish --otp="$(OTP)"

publish-wasm: platform-wasm
	[ ! -z "$(OTP)" ] && cd npm/esbuild-wasm && npm publish --otp="$(OTP)"

clean:
	rm -f esbuild npm/esbuild-wasm/esbuild.wasm npm/esbuild-wasm/wasm_exec.js
	rm -f npm/esbuild-windows-64/esbuild.exe
	rm -rf npm/esbuild-darwin-64/bin
	rm -rf npm/esbuild-linux-64/bin

################################################################################

github/test262:
	mkdir -p github
	git clone git@github.com:tc39/test262.git github/test262

demo/test262: | github/test262
	mkdir -p demo/test262
	cp -r github/test262/test demo/test262/test

################################################################################

test262: esbuild | demo/test262
	node scripts/test262.js

################################################################################

ROLLUP_CONFIG += import { terser } from 'rollup-plugin-terser';
ROLLUP_CONFIG += export default {
ROLLUP_CONFIG +=   output: { format: 'iife', name: 'THREE', sourcemap: true },
ROLLUP_CONFIG +=   plugins: [terser()],
ROLLUP_CONFIG += }

WEBPACK_FLAGS += --devtool=sourcemap
WEBPACK_FLAGS += --mode=production
WEBPACK_FLAGS += --output-library THREE

PARCEL_FLAGS += --global THREE
PARCEL_FLAGS += --no-autoinstall
PARCEL_FLAGS += --out-dir .
PARCEL_FLAGS += --public-url ./

FUSEBOX_RUN += require('fuse-box').fusebox({
FUSEBOX_RUN +=   target: 'browser',
FUSEBOX_RUN +=   entry: './fusebox-entry.js',
FUSEBOX_RUN +=   useSingleBundle: true,
FUSEBOX_RUN +=   output: './dist',
FUSEBOX_RUN += }).runProd();

node_modules:
	npm ci

################################################################################

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
	find bench/three -name '*.js' | xargs wc -l

################################################################################

demo-three: demo-three-esbuild demo-three-rollup demo-three-webpack demo-three-parcel

demo-three-esbuild: esbuild | demo/three
	rm -fr demo/three/esbuild
	mkdir -p demo/three/esbuild
	cd demo/three/esbuild && time -p ../../../esbuild --bundle --name=THREE --sourcemap --minify ../src/Three.js --outfile=Three.esbuild.js
	du -h demo/three/esbuild/Three.esbuild.js*

demo-three-rollup: | node_modules demo/three
	rm -fr demo/three/rollup
	mkdir -p demo/three/rollup
	echo "$(ROLLUP_CONFIG)" > demo/three/rollup/config.js
	cd demo/three/rollup && time -p ../../../node_modules/.bin/rollup ../src/Three.js -o Three.rollup.js -c config.js
	du -h demo/three/rollup/Three.rollup.js*

demo-three-webpack: | node_modules demo/three
	rm -fr demo/three/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p demo/three/webpack
	cd demo/three/webpack && time -p ../../../node_modules/.bin/webpack ../src/Three.js $(WEBPACK_FLAGS) -o Three.webpack.js
	du -h demo/three/webpack/Three.webpack.js*

demo-three-parcel: | node_modules demo/three
	rm -fr demo/three/parcel
	mkdir -p demo/three/parcel
	cd demo/three/parcel && time -p ../../../node_modules/.bin/parcel build ../src/Three.js $(PARCEL_FLAGS) --out-file Three.parcel.js
	du -h demo/three/parcel/Three.parcel.js*

demo-three-fusebox: | node_modules demo/three
	rm -fr demo/three/fusebox
	mkdir -p demo/three/fusebox
	echo "$(FUSEBOX_RUN)" > demo/three/fusebox/run.js
	echo 'import * as THREE from "../src/Three.js"; window.THREE = THREE' > demo/three/fusebox/fusebox-entry.js
	cd demo/three/fusebox && time -p node run.js
	du -h demo/three/fusebox/dist/app.js*

################################################################################

bench-three: bench-three-esbuild bench-three-rollup bench-three-webpack bench-three-parcel
	echo 'Line count:'
	find bench/three -name '*.js' | xargs wc -l | tail -n 1

bench-three-esbuild: esbuild | bench/three
	rm -fr bench/three/esbuild
	mkdir -p bench/three/esbuild
	cd bench/three/esbuild && time -p ../../../esbuild --bundle --name=THREE --sourcemap --minify ../entry.js --outfile=entry.esbuild.js
	du -h bench/three/esbuild/entry.esbuild.js*

bench-three-rollup: | node_modules bench/three
	rm -fr bench/three/rollup
	mkdir -p bench/three/rollup
	echo "$(ROLLUP_CONFIG)" > bench/three/rollup/config.js
	cd bench/three/rollup && time -p ../../../node_modules/.bin/rollup ../entry.js -o entry.rollup.js -c config.js
	du -h bench/three/rollup/entry.rollup.js*

bench-three-webpack: | node_modules bench/three
	rm -fr bench/three/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p bench/three/webpack
	cd bench/three/webpack && time -p ../../../node_modules/.bin/webpack ../entry.js $(WEBPACK_FLAGS) -o entry.webpack.js
	du -h bench/three/webpack/entry.webpack.js*

bench-three-parcel: | node_modules bench/three
	rm -fr bench/three/parcel
	mkdir -p bench/three/parcel
	cd bench/three/parcel && time -p ../../../node_modules/.bin/parcel build ../entry.js $(PARCEL_FLAGS) --out-file entry.parcel.js
	du -h bench/three/parcel/entry.parcel.js*

bench-three-fusebox: | node_modules bench/three
	rm -fr bench/three/fusebox
	mkdir -p bench/three/fusebox
	echo "$(FUSEBOX_RUN)" > bench/three/fusebox/run.js
	echo 'import * as THREE from "../entry.js"; window.THREE = THREE' > bench/three/fusebox/fusebox-entry.js
	cd bench/three/fusebox && time -p node --max-old-space-size=8192 run.js
	du -h bench/three/fusebox/dist/app.js*
