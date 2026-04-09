create table public.ws_music_tracks (
  id uuid not null default gen_random_uuid (),
  name text not null,
  release_date date not null default '2025-01-01'::date,
  url text not null,
  artist uuid not null,
  category uuid not null,
  album uuid null,
  featured boolean not null default false,
  lyrics text null,
  constraint ws_music_tracks_pkey primary key (id),
  constraint ws_music_tracks_album_fkey foreign KEY (album) references ws_music_albums (id) on update CASCADE on delete set null,
  constraint ws_music_tracks_artist_fkey foreign KEY (artist) references ws_music_artists (id) on update CASCADE on delete RESTRICT,
  constraint ws_music_tracks_category_fkey foreign KEY (category) references ws_music_categories (id) on update CASCADE on delete set null
) TABLESPACE pg_default;