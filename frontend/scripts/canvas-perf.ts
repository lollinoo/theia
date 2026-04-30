import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { runCanvasPerfBenchmark } from '../src/components/canvas/canvasPerfBenchmark';

interface CanvasPerfCliOptions {
  outputPath?: string;
  iterations?: number;
  warmupIterations?: number;
}

function readNumberOption(value: string | undefined, optionName: string): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(`${optionName} must be a non-negative integer`);
  }
  return parsed;
}

function parseArgs(args: string[]): CanvasPerfCliOptions {
  const options: CanvasPerfCliOptions = {};

  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === '--output') {
      options.outputPath = args[index + 1];
      index += 1;
      continue;
    }
    if (arg === '--iterations') {
      options.iterations = readNumberOption(args[index + 1], '--iterations');
      index += 1;
      continue;
    }
    if (arg === '--warmup') {
      options.warmupIterations = readNumberOption(args[index + 1], '--warmup');
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  return options;
}

async function main(): Promise<void> {
  const options = parseArgs(process.argv.slice(2));
  const result = runCanvasPerfBenchmark({
    iterations: options.iterations,
    warmupIterations: options.warmupIterations,
  });
  const json = `${JSON.stringify(result, null, 2)}\n`;

  if (options.outputPath) {
    const outputPath = resolve(process.cwd(), options.outputPath);
    await mkdir(dirname(outputPath), { recursive: true });
    await writeFile(outputPath, json, 'utf8');
  }

  process.stdout.write(json);
}

main().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  process.stderr.write(`${message}\n`);
  process.exitCode = 1;
});
