#!/usr/bin/env node

import { generateBinPath } from "./node-platform";
require('child_process').execFileSync(generateBinPath(), process.argv.slice(2), { stdio: 'inherit' });
