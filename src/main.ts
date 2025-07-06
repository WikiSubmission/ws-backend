import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
// import { Transport } from '@nestjs/microservices';
import { DocumentBuilder, SwaggerModule } from '@nestjs/swagger';
// import { ValidationPipe } from '@nestjs/common';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  // app.useGlobalPipes(
  //   new ValidationPipe({
  //     transform: true,
  //     transformOptions: { enableImplicitConversion: true },
  //     whitelist: true,
  //   }),
  // );

  const config = new DocumentBuilder()
    .setTitle('WikiSubmission API')
    .setDescription('API documentation of the WikiSubmission backend')
    .setVersion('0.0')
    .build();

  const document = SwaggerModule.createDocument(app, config);
  SwaggerModule.setup('docs', app, document);

  // app.connectMicroservice({
  //   transport: Transport.REDIS,
  //   options: { url: process.env.REDIS_URL || 'redis://localhost:6379' },
  // });

  await app.startAllMicroservices();
  await app.listen(process.env.PORT ?? 3000);
}
bootstrap();
