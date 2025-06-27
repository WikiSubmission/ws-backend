import { pathsToModuleNameMapper } from 'ts-jest';
import type { JestConfigWithTsJest } from 'ts-jest';
import tsconfig from './tsconfig.json';

const { compilerOptions } = tsconfig as {
  compilerOptions: {
    baseUrl?: string;
    paths?: Record<string, string[]>;
  };
};

const config: JestConfigWithTsJest = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  moduleFileExtensions: ['ts', 'js', 'json'],
  rootDir: './',
  testRegex: '.*\\.spec\\.ts$',
  modulePaths: ['<rootDir>'],
  moduleNameMapper: pathsToModuleNameMapper(compilerOptions.paths || {}, {
    prefix: '<rootDir>/',
  }),
  transform: {
    '^.+\\.(t|j)s$': 'ts-jest',
  },
  collectCoverageFrom: ['src/**/*.(t|j)s'],
  coverageDirectory: './coverage',
};

export default config;
