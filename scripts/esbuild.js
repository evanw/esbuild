const childProcess = require('child_process')
const path = require('path')
const fs = require('fs')

const repoDir = path.dirname(__dirname)
const npmDir = path.join(repoDir, 'npm', 'esbuild')

exports.installForTests = dir => {
  // Make sure esbuild is built
  childProcess.execSync('make', { cwd: repoDir, stdio: 'ignore' })

  // Create a fresh test directory
  childProcess.execSync(`rm -fr "${dir}"`)
  fs.mkdirSync(dir)

  // Install the "esbuild" package
  const env = { ...process.env, ESBUILD_BIN_PATH_FOR_TESTS: path.join(repoDir, 'esbuild') }
  const version = require(path.join(npmDir, 'package.json')).version
  fs.writeFileSync(path.join(dir, 'package.json'), '{}')
  console.log('Packing esbuild...')
  childProcess.execSync(`npm pack --silent "${npmDir}"`, { cwd: dir })
  console.log('Installing esbuild...')
  childProcess.execSync(`npm install --silent esbuild-${version}.tgz`, { cwd: dir, env })

  // Evaluate the code
  return require(path.join(dir, 'node_modules', 'esbuild'))
}
