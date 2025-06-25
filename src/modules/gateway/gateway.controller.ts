import { Controller, Get, Inject } from '@nestjs/common';
import { ClientProxy } from '@nestjs/microservices';

@Controller('gateway')
export class GatewayController {
  constructor(@Inject('USERS_SERVICE') private readonly client: ClientProxy) {}
  @Get()
  polo() {
    return { message: 'polo' };
  }
  @Get()
  getStatus() {
    return { message: 'Online.' };
  }
}
