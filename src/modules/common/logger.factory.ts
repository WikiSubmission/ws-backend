import { transports, format } from 'winston';
import { utilities as nestWinstonModuleUtilities } from 'nest-winston';

export const LoggerFactory = (appName: string) => {
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
      format.timestamp({ format: 'iso' }),
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
