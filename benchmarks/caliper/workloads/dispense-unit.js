'use strict';
const {createCoreWorkload} = require('./core-workload');
module.exports.createWorkloadModule = () => createCoreWorkload('dispense');
