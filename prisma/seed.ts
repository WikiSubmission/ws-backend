import { bootstrapQT } from './seeds/load-quran-other-translations-ws';
import { bootstrapW } from './seeds/load-quran-ws';
import { bootstrapWBW } from './seeds/load-quran-word-by-word';

async function main() {
  await bootstrapW();
  await bootstrapQT();
  await bootstrapWBW();
}

main()
  .then(() => {
    console.log('Seeding complete.');
    process.exit(0);
  })
  .catch((error) => {
    console.error('Seeding failed:', error);
    process.exit(1);
  });
