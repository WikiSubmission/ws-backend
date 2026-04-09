create table public.ws_music_albums (
  id uuid not null default gen_random_uuid (),
  name text not null,
  artist uuid not null,
  release_date date not null default '2025-01-01'::date,
  description text null,
  constraint ws_music_albums_pkey primary key (id),
  constraint ws_music_albums_artist_fkey foreign KEY (artist) references ws_music_artists (id) on update CASCADE on delete CASCADE
) TABLESPACE pg_default;