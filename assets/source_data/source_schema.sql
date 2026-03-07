CREATE TABLE IF NOT EXISTS public.ws_quran_chapters (
    chapter_number integer PRIMARY KEY, chapter_verses integer, revelation_order integer,
    title_english text, title_arabic text, title_transliterated text, title_turkish text,
    title_french text, title_german text, title_bahasa text, title_persian text,
    title_tamil text, title_swedish text, title_russian text, title_bengali text,
    title_urdu text, title_spanish text
);

CREATE TABLE IF NOT EXISTS public.ws_quran_index (
    verse_index integer, verse_id text PRIMARY KEY, chapter_number integer,
    verse_number integer, chapter_verses integer, verse_id_arabic text
);

CREATE TABLE IF NOT EXISTS public.ws_quran_text (
    verse_index integer, verse_id text PRIMARY KEY, english text, arabic text,
    transliterated text, arabic_clean text, chapter_number integer, verse_number integer,
    turkish text, french text, german text, bahasa text, persian text,
    tamil text, swedish text, russian text, bengali text, urdu text,
    persian_new text, spanish text
);

CREATE TABLE IF NOT EXISTS public.ws_quran_subtitles (
    verse_index integer, verse_id text PRIMARY KEY, english text, chapter_number integer,
    verse_number integer, turkish text, french text, german text, bahasa text,
    persian text, tamil text, swedish text, russian text, bengali text,
    spanish text, urdu text
);

CREATE TABLE IF NOT EXISTS public.ws_quran_word_by_word (
    index integer PRIMARY KEY, verse_index integer, verse_id text, word_index integer,
    root_word text, arabic text, english text, transliterated text, meanings text,
    chapter_number integer, verse_number integer
);

CREATE TABLE IF NOT EXISTS public.ws_quran_footnotes (
    verse_id text PRIMARY KEY, english text
);