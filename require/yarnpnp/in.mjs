console.log('Running Yarn PnP tests...')

import * as rd from 'react-dom'
if (rd.version !== '18.2.0') throw '❌ react-dom'

import * as s3 from 'strtok3'
import * as s3_core from 'strtok3/core'
if (!s3.fromFile) throw '❌ strtok3'
if (!s3_core.fromBuffer) throw '❌ strtok3/core'

import * as d3 from 'd3-time'
if (!d3.utcDay) throw '❌ d3-time'

import * as mm from 'mime'
if (mm.default.getType('txt') !== 'text/plain') throw '❌ mime'

import * as foo from 'foo'
if (foo.default !== 'foo') throw '❌ foo'

import * as bar from './bar/index.js'
if (bar.bar !== 'bar') throw '❌ bar'

console.log('✅ Yarn PnP tests passed')
