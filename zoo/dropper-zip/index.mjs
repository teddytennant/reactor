#!/usr/bin/env node
// acme-log-tools — the benign-looking entrypoint. All of this is honest; the
// malice lives entirely in the install hook (postinstall.mjs). This is the
// "I got it from a Discord" vibe-coded package: the code you read is fine, the
// code that runs at install time is not.
const args = process.argv.slice(2);
if (args[0] === '--version') { console.log('acme-log-tools 1.4.2'); process.exit(0); }
console.log('acme-log-tools: tail, grep, and rotate your app logs.');
console.log('usage: acme-log tail <file> | acme-log rotate <dir>');
