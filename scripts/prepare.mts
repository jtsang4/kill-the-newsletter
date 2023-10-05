import path from 'node:path';
import fs from 'node:fs/promises';
import css, { processCSS } from '@leafac/css';
import javascript from '@leafac/javascript';
import esbuild from 'esbuild';

const buildDir = path.resolve(process.cwd(), './build');

await fs.rm(buildDir, { recursive: true });

await fs.mkdir(buildDir, { recursive: true });

await fs.writeFile(path.resolve(buildDir, 'global.css'), processCSS(css``));

await fs.writeFile(
  path.resolve(buildDir, 'index.mts'),
  javascript`
    import "@fontsource/jetbrains-mono/variable.css";
    import "@fontsource/jetbrains-mono/variable-italic.css";

    import "tippy.js/dist/tippy.css";
    import "tippy.js/dist/svg-arrow.css";
    import "tippy.js/dist/border.css";
    import "@leafac/css/build/browser.css";
    import "./global.css";

    import tippy, * as tippyStatic from "tippy.js";
    window.tippy = tippy;
    window.tippy.hideAll = tippyStatic.hideAll;

    // TODO
    // import * as leafac from "@leafac/javascript/build/leafac--javascript.mjs";
    // window.leafac = leafac;
  `
);

const esbuildResult = await esbuild.build({
  entryPoints: [path.resolve(buildDir, 'index.mts')],
  outdir: path.resolve(buildDir, './static/'),
  entryNames: '[dir]/[name]--[hash]',
  assetNames: '[dir]/[name]--[hash]',

  loader: { '.woff2': 'file' },

  target: ['chrome100', 'safari14', 'edge100', 'firefox100', 'ios14'],

  bundle: true,
  minify: true,
  sourcemap: true,
  metafile: true,
});

await fs.unlink(path.resolve(buildDir, 'global.css'));
await fs.unlink(path.resolve(buildDir, 'index.mts'));

const paths: Record<string, string> = {};

for (const [javascriptBundle, { entryPoint, cssBundle }] of Object.entries(
  esbuildResult.metafile.outputs
)) {
  if (entryPoint === 'build/index.mts' && typeof cssBundle === 'string') {
    paths['index.css'] = cssBundle.slice('build/static/'.length);
    paths['index.mts'] = javascriptBundle.slice('build/static/'.length);
    break;
  }
}

await fs.writeFile(
  path.resolve(buildDir, 'static/paths.json'),
  JSON.stringify(paths, undefined, 2)
);

for (const source of ['static/favicon.ico']) {
  const destination = path.join(buildDir, source);
  await fs.mkdir(path.dirname(destination), { recursive: true });
  await fs.copyFile(source, destination);
}
