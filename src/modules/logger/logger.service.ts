import { Injectable, LogLevel } from '@nestjs/common';
import { format, createLogger, transports } from 'winston';
import { utilities as nestWinstonModuleUtilities } from 'nest-winston';
import { LoggerService as LoggerServiceNest } from '@nestjs/common';

export const getLoggerConfigs = (appName: string) => {
  let consoleFormat;

  const DEBUG = process.env.DEBUG;
  const USE_JSON_LOGGER = process.env.USE_JSON_LOGGER;

  if (USE_JSON_LOGGER === 'true') {
    consoleFormat = format.combine(
      format.ms(),
      format.timestamp(),
      format.json(),
    );
  } else if (process.env.NODE_ENV === 'test') {
    return { silent: true };
  } else {
    consoleFormat = format.combine(
      format.timestamp(),
      format.ms(),
      nestWinstonModuleUtilities.format.nestLike(appName, {
        colors: true,
        prettyPrint: true,
      }),
    );
  }

  return {
    level: DEBUG ? 'debug' : 'info',
    transports: [
      new transports.Console({ format: consoleFormat }),
      new transports.File({
        filename: 'logs/error.log',
        level: 'error',
        format: format.combine(format.timestamp(), format.json()),
      }),
    ],
  };
};

const logger = createLogger(getLoggerConfigs('ws-quran'));

@Injectable()
export class LoggerService implements LoggerServiceNest {
  log(message: any, context?: string, ...optionalParams: any[]) {
    logger.log('info', message, ...optionalParams);
  }
  error(message: any, ...optionalParams: any[]) {
    logger.error(message, ...optionalParams);
  }
  warn(message: any, ...optionalParams: any[]) {
    logger.warn(message, ...optionalParams);
  }
  debug(message: any, ...optionalParams: any[]) {
    logger.debug(message, ...optionalParams);
  }
  verbose?(message: any, ...optionalParams: any[]) {
    logger.verbose(message, ...optionalParams);
  }
  fatal?(message: any, ...optionalParams: any[]) {
    logger.error(message, ...optionalParams);
  }
  setLogLevels?(levels: LogLevel[]) {
    if (levels && levels.length > 0) {
      const level = levels[0]; // Default to the first level in the array
      logger.level = level;
      logger.info(`Log level set to ${level}`);
    }
  }
}
