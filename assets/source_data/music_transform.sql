-- Transform music source tables (UUID PKs) into app tables (INT IDENTITY PKs).
-- Must run after ws_music_artists, ws_music_categories, and ws_music_tracks
-- source tables have been imported.

-- Step 1: Artists
INSERT INTO public.music_artists (name, description, image_url, display_priority)
SELECT name, description, image_url, display_priority
FROM public.ws_music_artists
ORDER BY display_priority ASC, name ASC
ON CONFLICT (name) DO UPDATE SET
    description      = EXCLUDED.description,
    image_url        = EXCLUDED.image_url,
    display_priority = EXCLUDED.display_priority;

-- Step 2: Categories
INSERT INTO public.music_categories (name, description, display_priority)
SELECT name, description, display_priority
FROM public.ws_music_categories
ORDER BY display_priority ASC, name ASC
ON CONFLICT (name) DO UPDATE SET
    description      = EXCLUDED.description,
    display_priority = EXCLUDED.display_priority;

-- Step 3: Tracks — resolve UUID FKs via name-join into INT app IDs
INSERT INTO public.music_tracks (name, release_date, url, artist_id, category_id, album_id, featured, lyrics)
SELECT
    t.name,
    t.release_date,
    t.url,
    ma.id     AS artist_id,
    mc.id     AS category_id,
    NULL::int AS album_id,
    t.featured,
    t.lyrics
FROM public.ws_music_tracks       t
JOIN public.ws_music_artists      wa ON wa.id = t.artist
JOIN public.music_artists         ma ON ma.name = wa.name
JOIN public.ws_music_categories   wc ON wc.id = t.category
JOIN public.music_categories      mc ON mc.name = wc.name
ON CONFLICT DO NOTHING;
