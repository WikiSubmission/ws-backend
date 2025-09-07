Besides the migrations and the seeding that would need to be applied through `prisma migrate dev`, there are some additional steps that need to be taken to ensure that the database is properly set up and ready to use. These steps include:

1. Asserting that the added_fts migrations have been applied successfully. It only works directly with postgres, not prisma.
2. Applying the migrations to the database.
3. Seeding the database with initial data.
