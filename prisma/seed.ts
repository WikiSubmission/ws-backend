import { bootstrapQT } from 'src/scripts/load-quran-other-translations-ws';
import { bootstrapW } from 'src/scripts/load-quran-ws';
import { bootstrapWBW } from 'src/scripts/load-quran-word-by-word';

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
