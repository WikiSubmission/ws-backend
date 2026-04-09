create table public.ws_music_categories (
  id uuid not null default gen_random_uuid (),
  name text not null,
  description text null,
  display_priority integer not null default 0,
  constraint ws_music_categories_pkey primary key (id),
  constraint ws_music_categories_name_key unique (name)
) TABLESPACE pg_default;