#!/usr/bin/env node

import { extractedBinPath } from "./node-platform";
require('child_process').execFileSync(extractedBinPath(), process.argv.slice(2), { stdio: 'inherit' });
