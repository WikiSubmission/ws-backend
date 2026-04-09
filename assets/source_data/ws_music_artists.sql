create table public.ws_music_artists (
  id uuid not null default gen_random_uuid (),
  name text not null,
  description text null,
  image_url text not null default 'https://cdn.wikisubmission.org/media/brand-assets/logo-transparent.png'::text,
  display_priority integer not null default 0,
  constraint ws_music_artists_pkey primary key (id),
  constraint ws_music_artists_name_key unique (name)
) TABLESPACE pg_default;