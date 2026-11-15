-- 001_baseline.sql
-- Squashed schema baseline, generated from Railway production via pg_dump.
-- Idempotent: safe to run against fresh or already-migrated databases.
-- Replaces the original 13 numbered migrations (preserved in git history).

--

--

--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

--
-- Name: vector; Type: EXTENSION; Schema: -; Owner: -
--

-- pgvector is optional: the desktop build bundles a Postgres without it, and
-- semantic search is off there anyway. The embedding column and its index are
-- added further down only when the extension is installed.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector') THEN
        CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
    END IF;
END $$;

--
-- Name: translation_status; Type: TYPE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    CREATE TYPE public.translation_status AS ENUM (
    'draft',
    'published',
    'archived'
);
EXCEPTION WHEN duplicate_object THEN NULL;
END $WS_BASELINE$;

--
-- Name: user_role; Type: TYPE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    CREATE TYPE public.user_role AS ENUM (
    'admin',
    'editor',
    'member'
);
EXCEPTION WHEN duplicate_object THEN NULL;
END $WS_BASELINE$;

--
-- Name: abjadi_rank(text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE OR REPLACE FUNCTION public.abjadi_rank(letters text) RETURNS integer
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    AS $$
  WITH ranks(ch, n) AS (
    VALUES
      ('ا', 1),  ('ب', 2),  ('ج', 3),  ('د', 4),
      ('ه', 5),  ('و', 6),  ('ز', 7),  ('ح', 8),
      ('ط', 9),  ('ي', 10), ('ك', 11), ('ل', 12),
      ('م', 13), ('ن', 14), ('س', 15), ('ع', 16),
      ('ف', 17), ('ص', 18), ('ق', 19), ('ر', 20),
      ('ش', 21), ('ت', 22), ('ث', 23), ('خ', 24),
      ('ذ', 25), ('ض', 26), ('ظ', 27), ('غ', 28)
  ),
  parts AS (
    SELECT string_to_array(letters, ' ') AS arr
  )
  SELECT
      COALESCE((SELECT n FROM ranks WHERE ch = arr[1]), 0) * 1000
    + COALESCE((SELECT n FROM ranks WHERE ch = arr[2]), 0) * 30
    + COALESCE((SELECT n FROM ranks WHERE ch = arr[3]), 0)
  FROM parts;
$$;

--
-- Name: appendices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.appendices (
    id integer NOT NULL,
    lang_id smallint NOT NULL,
    code smallint,
    title text,
    snippet text
);

--
-- Name: appendices_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.appendices ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.appendices_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_book_text; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.bible_book_text (
    id smallint NOT NULL,
    book_id smallint NOT NULL,
    lang_id smallint NOT NULL,
    name text NOT NULL
);

--
-- Name: bible_book_text_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.bible_book_text ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.bible_book_text_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_books; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.bible_books (
    id smallint NOT NULL,
    book_number smallint NOT NULL,
    name text NOT NULL,
    testament character varying(2) DEFAULT 'OT'::character varying NOT NULL,
    CONSTRAINT bible_books_testament_check CHECK (((testament)::text = ANY ((ARRAY['OT'::character varying, 'NT'::character varying])::text[])))
);

--
-- Name: bible_books_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.bible_books ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.bible_books_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_chapters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.bible_chapters (
    id integer NOT NULL,
    book_id smallint NOT NULL,
    chapter_number smallint NOT NULL,
    verse_count integer DEFAULT 0 NOT NULL
);

--
-- Name: bible_chapters_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.bible_chapters ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.bible_chapters_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verse_text; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.bible_verse_text (
    id integer NOT NULL,
    verse_id integer NOT NULL,
    lang_id smallint NOT NULL,
    content text,
    footer text,
    theo_footnotes text[],
    content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, ((COALESCE(content, ''::text) || ' '::text) || COALESCE(footer, ''::text)))) STORED,
    published_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

--
-- Name: bible_verse_text_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.bible_verse_text ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.bible_verse_text_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.bible_verses (
    id integer NOT NULL,
    chapter_id integer NOT NULL,
    verse_number smallint NOT NULL
);

--
-- Name: bible_verses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.bible_verses ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.bible_verses_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.chapter_text (
    id integer NOT NULL,
    lang_id smallint NOT NULL,
    chapter_id integer NOT NULL,
    title text,
    published_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    title_tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, COALESCE(title, ''::text))) STORED
);

--
-- Name: chapter_text_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.chapter_text_drafts (
    id integer NOT NULL,
    chapter_id integer NOT NULL,
    lang_id smallint NOT NULL,
    title text,
    version integer DEFAULT 1,
    status public.translation_status DEFAULT 'draft'::public.translation_status,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    author_id integer
);

--
-- Name: chapter_text_drafts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.chapter_text_drafts ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.chapter_text_drafts_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.chapter_text_history (
    id integer NOT NULL,
    chapter_id integer NOT NULL,
    chapter_text_id integer NOT NULL,
    lang_id smallint NOT NULL,
    title text,
    version integer,
    archived_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

--
-- Name: chapter_text_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.chapter_text_history ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.chapter_text_history_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.chapter_text ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.chapter_text_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.chapters (
    id integer NOT NULL,
    chapter_number integer NOT NULL,
    verse_count integer NOT NULL,
    revelation_order integer NOT NULL
);

--
-- Name: chapters_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.chapters ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.chapters_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: languages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.languages (
    id smallint NOT NULL,
    code character varying(10) NOT NULL,
    name character varying(50) NOT NULL,
    direction character varying(3) DEFAULT 'ltr'::character varying
);

--
-- Name: languages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.languages ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.languages_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_albums; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.music_albums (
    id integer NOT NULL,
    name text NOT NULL,
    artist_id integer NOT NULL,
    release_date date DEFAULT '2025-01-01'::date NOT NULL,
    description text
);

--
-- Name: music_albums_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.music_albums ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.music_albums_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_artists; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.music_artists (
    id integer NOT NULL,
    name text NOT NULL,
    description text,
    image_url text DEFAULT 'https://cdn.wikisubmission.org/media/brand-assets/logo-transparent.png'::text NOT NULL,
    display_priority integer DEFAULT 0 NOT NULL
);

--
-- Name: music_artists_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.music_artists ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.music_artists_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.music_categories (
    id integer NOT NULL,
    name text NOT NULL,
    description text,
    display_priority integer DEFAULT 0 NOT NULL
);

--
-- Name: music_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.music_categories ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.music_categories_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_tracks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.music_tracks (
    id integer NOT NULL,
    name text NOT NULL,
    release_date date DEFAULT '2025-01-01'::date NOT NULL,
    url text NOT NULL,
    artist_id integer NOT NULL,
    category_id integer,
    album_id integer,
    featured boolean DEFAULT false NOT NULL,
    lyrics text
);

--
-- Name: music_tracks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.music_tracks ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.music_tracks_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.word_text (
    id integer NOT NULL,
    word_id integer NOT NULL,
    lang_id smallint NOT NULL,
    word text,
    word_root text,
    meaning text,
    published_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    word_tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, ((COALESCE(word, ''::text) || ' '::text) || COALESCE(meaning, ''::text)))) STORED
);

--
-- Name: root_index_mv; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW IF NOT EXISTS public.root_index_mv AS
 SELECT word_root AS letters,
    (count(*))::integer AS occurrences,
    public.abjadi_rank(word_root) AS abjadi_rank
   FROM public.word_text
  WHERE ((word_root IS NOT NULL) AND (word_root <> ''::text))
  GROUP BY word_root
  WITH NO DATA;

--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.users (
    id integer NOT NULL,
    supabase_uid uuid NOT NULL,
    email text NOT NULL,
    display_name text,
    role public.user_role DEFAULT 'member'::public.user_role NOT NULL,
    permissions jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.users ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.users_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.verse_text (
    id integer NOT NULL,
    verse_id integer NOT NULL,
    lang_id smallint NOT NULL,
    subtitle text,
    content text,
    footer text,
    published_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, ((((COALESCE(content, ''::text) || ' '::text) || COALESCE(subtitle, ''::text)) || ' '::text) || COALESCE(footer, ''::text)))) STORED
);

--
-- Name: verse_text_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.verse_text_drafts (
    id integer NOT NULL,
    verse_id integer NOT NULL,
    lang_id smallint NOT NULL,
    subtitle text,
    content text,
    footer text,
    version integer DEFAULT 1,
    status public.translation_status DEFAULT 'draft'::public.translation_status,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    author_id integer
);

--
-- Name: verse_text_drafts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.verse_text_drafts ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.verse_text_drafts_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.verse_text_history (
    id integer NOT NULL,
    verse_text_id integer NOT NULL,
    verse_id integer NOT NULL,
    lang_id smallint NOT NULL,
    content text,
    subtitle text,
    footer text,
    version integer,
    archived_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    reason text
);

--
-- Name: verse_text_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.verse_text_history ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.verse_text_history_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.verse_text ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.verse_text_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.verses (
    id integer NOT NULL,
    chapter_id integer,
    verse_index integer
);

--
-- Name: verses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.verses ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.verses_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.word_text_drafts (
    id integer NOT NULL,
    word_id integer NOT NULL,
    lang_id smallint NOT NULL,
    word text,
    word_root text,
    meaning text,
    version integer DEFAULT 1,
    status public.translation_status DEFAULT 'draft'::public.translation_status,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    author_id integer
);

--
-- Name: word_text_drafts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.word_text_drafts ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.word_text_drafts_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.word_text_history (
    id integer NOT NULL,
    word_id integer NOT NULL,
    word_text_id integer NOT NULL,
    lang_id smallint NOT NULL,
    word text,
    word_root text,
    meaning text,
    version integer,
    archived_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

--
-- Name: word_text_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.word_text_history ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.word_text_history_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.word_text ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.word_text_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: words; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.words (
    id integer NOT NULL,
    verse_id integer,
    global_index integer,
    word_index integer
);

--
-- Name: words_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE public.words ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
        SEQUENCE NAME public.words_id_seq
        START WITH 1
        INCREMENT BY 1
        NO MINVALUE
        NO MAXVALUE
        CACHE 1
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: appendices appendices_lang_code_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.appendices
        ADD CONSTRAINT appendices_lang_code_unique UNIQUE (lang_id, code);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: appendices appendices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.appendices
        ADD CONSTRAINT appendices_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_book_text bible_book_text_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_book_text
        ADD CONSTRAINT bible_book_text_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_books bible_books_book_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_books
        ADD CONSTRAINT bible_books_book_number_key UNIQUE (book_number);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_books bible_books_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_books
        ADD CONSTRAINT bible_books_name_key UNIQUE (name);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_books bible_books_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_books
        ADD CONSTRAINT bible_books_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_chapters bible_chapters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_chapters
        ADD CONSTRAINT bible_chapters_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verse_text bible_verse_text_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_verse_text
        ADD CONSTRAINT bible_verse_text_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verses bible_verses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_verses
        ADD CONSTRAINT bible_verses_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_drafts chapter_text_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text_drafts
        ADD CONSTRAINT chapter_text_drafts_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_history chapter_text_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text_history
        ADD CONSTRAINT chapter_text_history_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text chapter_text_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text
        ADD CONSTRAINT chapter_text_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapters chapters_chapter_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapters
        ADD CONSTRAINT chapters_chapter_number_key UNIQUE (chapter_number);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapters chapters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapters
        ADD CONSTRAINT chapters_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: languages languages_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.languages
        ADD CONSTRAINT languages_code_key UNIQUE (code);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: languages languages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.languages
        ADD CONSTRAINT languages_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_albums music_albums_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_albums
        ADD CONSTRAINT music_albums_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_artists music_artists_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_artists
        ADD CONSTRAINT music_artists_name_key UNIQUE (name);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_artists music_artists_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_artists
        ADD CONSTRAINT music_artists_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_categories music_categories_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_categories
        ADD CONSTRAINT music_categories_name_key UNIQUE (name);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_categories music_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_categories
        ADD CONSTRAINT music_categories_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_tracks music_tracks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_tracks
        ADD CONSTRAINT music_tracks_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_book_text uq_bible_book_text_lang; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_book_text
        ADD CONSTRAINT uq_bible_book_text_lang UNIQUE (book_id, lang_id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_chapters uq_bible_chapter; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_chapters
        ADD CONSTRAINT uq_bible_chapter UNIQUE (book_id, chapter_number);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verses uq_bible_verse; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_verses
        ADD CONSTRAINT uq_bible_verse UNIQUE (chapter_id, verse_number);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verse_text uq_bible_verse_text_lang; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_verse_text
        ADD CONSTRAINT uq_bible_verse_text_lang UNIQUE (verse_id, lang_id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text uq_chapter_text_lang; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text
        ADD CONSTRAINT uq_chapter_text_lang UNIQUE (chapter_id, lang_id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text uq_verse_text_lang; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text
        ADD CONSTRAINT uq_verse_text_lang UNIQUE (verse_id, lang_id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verses uq_verses_chapter_verse; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verses
        ADD CONSTRAINT uq_verses_chapter_verse UNIQUE (chapter_id, verse_index);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text uq_word_text_lang; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text
        ADD CONSTRAINT uq_word_text_lang UNIQUE (word_id, lang_id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: words uq_words_verse_word; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.words
        ADD CONSTRAINT uq_words_verse_word UNIQUE (verse_id, word_index);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.users
        ADD CONSTRAINT users_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: users users_supabase_uid_key; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.users
        ADD CONSTRAINT users_supabase_uid_key UNIQUE (supabase_uid);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_drafts verse_text_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text_drafts
        ADD CONSTRAINT verse_text_drafts_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_history verse_text_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text_history
        ADD CONSTRAINT verse_text_history_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text verse_text_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text
        ADD CONSTRAINT verse_text_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verses verses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verses
        ADD CONSTRAINT verses_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_drafts word_text_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text_drafts
        ADD CONSTRAINT word_text_drafts_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_history word_text_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text_history
        ADD CONSTRAINT word_text_history_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text word_text_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text
        ADD CONSTRAINT word_text_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: words words_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.words
        ADD CONSTRAINT words_pkey PRIMARY KEY (id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: idx_bible_chapters_book; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_bible_chapters_book ON public.bible_chapters USING btree (book_id, chapter_number);

--
-- Name: idx_bible_verse_content_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_bible_verse_content_trgm ON public.bible_verse_text USING gin (content public.gin_trgm_ops);

--
-- Name: idx_bible_verse_fts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_bible_verse_fts ON public.bible_verse_text USING gin (content_tsv);

--
-- Name: idx_bible_verse_text_lang; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_bible_verse_text_lang ON public.bible_verse_text USING btree (verse_id, lang_id);

--
-- Name: idx_bible_verses_nav; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_bible_verses_nav ON public.bible_verses USING btree (chapter_id, verse_number);

--
-- Name: idx_chapter_text_fts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_chapter_text_fts ON public.chapter_text USING gin (title_tsv);

--
-- Name: idx_chapter_text_lang; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_chapter_text_lang ON public.chapter_text USING btree (chapter_id, lang_id);

--
-- Name: idx_chapter_text_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_chapter_text_trgm ON public.chapter_text USING gin (title public.gin_trgm_ops);

--
-- Name: idx_chapters_number; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS idx_chapters_number ON public.chapters USING btree (chapter_number);

--
-- Name: idx_music_albums_artist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_music_albums_artist ON public.music_albums USING btree (artist_id);

--
-- Name: idx_music_tracks_album; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_music_tracks_album ON public.music_tracks USING btree (album_id);

--
-- Name: idx_music_tracks_artist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_music_tracks_artist ON public.music_tracks USING btree (artist_id);

--
-- Name: idx_music_tracks_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_music_tracks_category ON public.music_tracks USING btree (category_id);

--
-- Name: idx_music_tracks_featured; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_music_tracks_featured ON public.music_tracks USING btree (featured) WHERE (featured = true);

--
-- Name: idx_root_index_mv_abjadi; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_root_index_mv_abjadi ON public.root_index_mv USING btree (abjadi_rank);

--
-- Name: idx_root_index_mv_letters; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS idx_root_index_mv_letters ON public.root_index_mv USING btree (letters);

--
-- Name: idx_root_index_mv_letters_pattern; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_root_index_mv_letters_pattern ON public.root_index_mv USING btree (letters text_pattern_ops);

--
-- Name: idx_root_index_mv_occurrences; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_root_index_mv_occurrences ON public.root_index_mv USING btree (occurrences DESC);

--
-- Name: idx_users_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_users_role ON public.users USING btree (role);

--
-- Name: idx_users_supabase_uid; Type: INDEX; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'users'
          AND column_name = 'supabase_uid'
    ) THEN
        CREATE INDEX IF NOT EXISTS idx_users_supabase_uid ON public.users USING btree (supabase_uid);
    END IF;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: idx_verse_content_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_verse_content_trgm ON public.verse_text USING gin (content public.gin_trgm_ops);

--
-- Name: idx_verse_drafts_nav; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_verse_drafts_nav ON public.verse_text_drafts USING btree (verse_id, lang_id, status);

--
-- Name: idx_verse_fts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_verse_fts ON public.verse_text USING gin (content_tsv);

--
-- Name: idx_verse_history_nav; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_verse_history_nav ON public.verse_text_history USING btree (verse_id, lang_id, archived_at);

--
-- Name: idx_verse_text_embedding_hnsw; Type: INDEX; Schema: public; Owner: -
--

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        ALTER TABLE public.verse_text ADD COLUMN IF NOT EXISTS embedding public.vector(512);
        CREATE INDEX IF NOT EXISTS idx_verse_text_embedding_hnsw ON public.verse_text USING hnsw (embedding public.vector_cosine_ops) WITH (m='16', ef_construction='128');
    END IF;
END $$;

--
-- Name: idx_verse_text_lang; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_verse_text_lang ON public.verse_text USING btree (verse_id, lang_id);

--
-- Name: idx_verses_nav; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_verses_nav ON public.verses USING btree (chapter_id, verse_index);

ALTER TABLE public.verses CLUSTER ON idx_verses_nav;

--
-- Name: idx_word_fts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_word_fts ON public.word_text USING gin (word_tsv);

--
-- Name: idx_word_meaning_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_word_meaning_lookup ON public.word_text USING btree (meaning) WHERE (meaning IS NOT NULL);

--
-- Name: idx_word_root_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_word_root_lookup ON public.word_text USING btree (word_root) WHERE (word_root IS NOT NULL);

--
-- Name: idx_word_text_lang; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_word_text_lang ON public.word_text USING btree (word_id, lang_id);

--
-- Name: idx_word_text_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_word_text_trgm ON public.word_text USING gin (word public.gin_trgm_ops);

--
-- Name: idx_words_nav; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_words_nav ON public.words USING btree (verse_id, word_index);

--
-- Name: appendices appendices_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.appendices
        ADD CONSTRAINT appendices_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_book_text bible_book_text_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_book_text
        ADD CONSTRAINT bible_book_text_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verse_text bible_verse_text_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_verse_text
        ADD CONSTRAINT bible_verse_text_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_drafts chapter_text_drafts_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text_drafts
        ADD CONSTRAINT chapter_text_drafts_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_history chapter_text_history_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text_history
        ADD CONSTRAINT chapter_text_history_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text chapter_text_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text
        ADD CONSTRAINT chapter_text_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_chapters fk_bible_book; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_chapters
        ADD CONSTRAINT fk_bible_book FOREIGN KEY (book_id) REFERENCES public.bible_books(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_book_text fk_bible_book_text; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_book_text
        ADD CONSTRAINT fk_bible_book_text FOREIGN KEY (book_id) REFERENCES public.bible_books(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verses fk_bible_chapter; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_verses
        ADD CONSTRAINT fk_bible_chapter FOREIGN KEY (chapter_id) REFERENCES public.bible_chapters(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: bible_verse_text fk_bible_verse; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.bible_verse_text
        ADD CONSTRAINT fk_bible_verse FOREIGN KEY (verse_id) REFERENCES public.bible_verses(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text fk_chapter; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text
        ADD CONSTRAINT fk_chapter FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_drafts fk_chapter; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text_drafts
        ADD CONSTRAINT fk_chapter FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_history fk_chapter; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text_history
        ADD CONSTRAINT fk_chapter FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verses fk_chapter; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verses
        ADD CONSTRAINT fk_chapter FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: chapter_text_history fk_chapter_text; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.chapter_text_history
        ADD CONSTRAINT fk_chapter_text FOREIGN KEY (chapter_text_id) REFERENCES public.chapter_text(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text fk_verse; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text
        ADD CONSTRAINT fk_verse FOREIGN KEY (verse_id) REFERENCES public.verses(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_drafts fk_verse; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text_drafts
        ADD CONSTRAINT fk_verse FOREIGN KEY (verse_id) REFERENCES public.verses(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_history fk_verse; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text_history
        ADD CONSTRAINT fk_verse FOREIGN KEY (verse_id) REFERENCES public.verses(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: words fk_verse; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.words
        ADD CONSTRAINT fk_verse FOREIGN KEY (verse_id) REFERENCES public.verses(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_history fk_verse_text; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text_history
        ADD CONSTRAINT fk_verse_text FOREIGN KEY (verse_text_id) REFERENCES public.verse_text(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text fk_word; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text
        ADD CONSTRAINT fk_word FOREIGN KEY (word_id) REFERENCES public.words(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_drafts fk_word; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text_drafts
        ADD CONSTRAINT fk_word FOREIGN KEY (word_id) REFERENCES public.words(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_history fk_word; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text_history
        ADD CONSTRAINT fk_word FOREIGN KEY (word_id) REFERENCES public.words(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_history fk_word_text; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text_history
        ADD CONSTRAINT fk_word_text FOREIGN KEY (word_text_id) REFERENCES public.word_text(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_albums music_albums_artist_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_albums
        ADD CONSTRAINT music_albums_artist_id_fkey FOREIGN KEY (artist_id) REFERENCES public.music_artists(id) ON UPDATE CASCADE ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_tracks music_tracks_album_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_tracks
        ADD CONSTRAINT music_tracks_album_id_fkey FOREIGN KEY (album_id) REFERENCES public.music_albums(id) ON UPDATE CASCADE ON DELETE SET NULL;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_tracks music_tracks_artist_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_tracks
        ADD CONSTRAINT music_tracks_artist_id_fkey FOREIGN KEY (artist_id) REFERENCES public.music_artists(id) ON UPDATE CASCADE ON DELETE RESTRICT;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: music_tracks music_tracks_category_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.music_tracks
        ADD CONSTRAINT music_tracks_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.music_categories(id) ON UPDATE CASCADE ON DELETE SET NULL;
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_drafts verse_text_drafts_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text_drafts
        ADD CONSTRAINT verse_text_drafts_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text_history verse_text_history_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text_history
        ADD CONSTRAINT verse_text_history_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: verse_text verse_text_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.verse_text
        ADD CONSTRAINT verse_text_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_drafts word_text_drafts_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text_drafts
        ADD CONSTRAINT word_text_drafts_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text_history word_text_history_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text_history
        ADD CONSTRAINT word_text_history_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--
-- Name: word_text word_text_lang_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

DO $WS_BASELINE$ BEGIN
    ALTER TABLE ONLY public.word_text
        ADD CONSTRAINT word_text_lang_id_fkey FOREIGN KEY (lang_id) REFERENCES public.languages(id);
EXCEPTION WHEN OTHERS THEN NULL;
END $WS_BASELINE$;

--

--



-- ---------------------------------------------------------------------------
-- Seed: languages + appendices (preserved from legacy 003_seed.sql)
-- ---------------------------------------------------------------------------

INSERT INTO languages (code, name, direction) VALUES 
('ar', 'Arabic', 'rtl'),
('ac', 'Arabic Clean', 'rtl'),
('en', 'English', 'ltr'),
('fr', 'French', 'ltr'),
('tr', 'Turkish', 'ltr'),
('de', 'German', 'ltr'),
('id', 'Bahasa', 'ltr'),
('fa', 'Persian', 'rtl'),
('ta', 'Tamil', 'ltr'),
('sv', 'Swedish', 'ltr'),
('ru', 'Russian', 'ltr'),
('bn', 'Bengali', 'ltr'),
('ur', 'Urdu', 'rtl'),
('es', 'Spanish', 'ltr'),
('tl', 'Transliterated', 'ltr')
ON CONFLICT (code) DO NOTHING;

INSERT INTO appendices (lang_id, code, title, snippet) VALUES 
((SELECT id FROM languages WHERE code = 'en'),1, 'One of the Great Miracles [74:35]', 'Every element of the Quran is mathematically composed - the suras, the verses, the words, the number of certain letters, the number of words from the same root, the number and variety of divine names, the unique spelling of certain words, the absence or deliberate alteration of certain letters within certain words, and many other elements of the Quran besides its content...'), ((SELECT id FROM languages WHERE code = 'en'),2, 'God''s Messenger of the Covenant [3:81]', 'His mission is to purify and unify all existing religions into one: Islam (Submission)...'), ((SELECT id FROM languages WHERE code = 'en'),3, 'We Made the Quran Easy [54:17]', 'Memorization of the Quran has played a vital role in preserving the original text generation after generation at a time when written books were a rarity...'), ((SELECT id FROM languages WHERE code = 'en'),4, 'Why Was the Quran Revealed in Arabic?', 'We learn from 41:44 that the sincere believers have access to the Quran, regardless of their mother tongue...'), ((SELECT id FROM languages WHERE code = 'en'),5, 'Heaven and Hell', 'The descriptions of Heaven and Hell throughout the Quran are allegorical...'), ((SELECT id FROM languages WHERE code = 'en'),6, 'Greatness of God', 'We learn from Verse 39:67 that God''s greatness is far beyond human comprehension - the verse states that all seven universes are ''folded within God''s hand.''...'), ((SELECT id FROM languages WHERE code = 'en'),7, 'Why Were We Created?', 'We are in this world because we committed a horrendous crime, and this life is our chance to redeem ourselves, denounce our crime, and rejoin God''s kingdom...'), ((SELECT id FROM languages WHERE code = 'en'),8, 'The Myth of Intercession', 'To believe that anyone, other than God, can intercede on our behalf to have our sins forgiven or our wishes fulfilled, is to set up partners with God. This is idolatry...'), ((SELECT id FROM languages WHERE code = 'en'),9, 'Abraham: Original Messenger of Submission', 'One of the prevalent myths is that Muhammad was the founder of Islam...'), ((SELECT id FROM languages WHERE code = 'en'),10, 'God''s Usage of the Plural Tense', 'Whenever the first person plural form is used by the Almighty, it invariably indicates participation of other entities, such as the angels...'), ((SELECT id FROM languages WHERE code = 'en'),11, 'The Day of Resurrection', 'The earth will shine with the light of God (39:69) as He comes to our universe, together with the angels (89:22)...'), ((SELECT id FROM languages WHERE code = 'en'),12, 'Role of the Prophet Muhammad', 'The Prophet''s sole mission was to deliver Quran, the whole Quran, and nothing but Quran (3:20; 5:48-50, 5:92, 5:99; 6:19; 13:40; 16:35, 16:82; 24:54; 29:18; 42:48; 64:12)...'), ((SELECT id FROM languages WHERE code = 'en'),13, 'The First Pillar of Submission', 'Verse 3:18 states the First Pillar of Islam (Submission): ''God bears witness that there is no other god besides Him, and so do the angelsand those who possess knowledge.''...'), ((SELECT id FROM languages WHERE code = 'en'),14, 'Predestination', 'God fully knows what kind of decision each of us is destined to make; He knows which of us are going to Heaven and which are going to Hell...'), ((SELECT id FROM languages WHERE code = 'en'),15, 'Religious Duties: Gift from God', 'When Abraham implored God in 14:40, he did not ask for wealth or health; the gift he implored for was: ''Please God, make me one who observes the contact prayers (Salat).''...'), ((SELECT id FROM languages WHERE code = 'en'),16, 'Dietary Prohibition', 'The Quran teaches that God is extremely displeased with those who prohibit anything that was not specifically prohibited in the Quran (16:112-116)...'), ((SELECT id FROM languages WHERE code = 'en'),17, 'Death', 'At the moment of death, everyone knows his or her destiny; Heaven or Hell....'), ((SELECT id FROM languages WHERE code = 'en'),18, 'Quran is All You Need', 'The Quran proclaims that the Quran is complete, perfect, and fully detailed (6:19, 38, 114, 115; 50:45), and that religious regulations not specifically instituted in the Quran constitute a religion other than Islam, i.e., Submission (42:21, 17:46)...'), ((SELECT id FROM languages WHERE code = 'en'),19, 'Hadith and Sunna: Satanic Innovations', 'The Quran informs us that some enemies of the Prophet, described as ''human and jinn devils,'' will fabricate lies and attribute them to the Prophet (6:112, 25:31)...'), ((SELECT id FROM languages WHERE code = 'en'),20, 'Quran: Unlike Any Other Book', 'The Quran is God''s Final Testament to the world, and He has pledged to protect it from the slightest distortion (15:9)...'), ((SELECT id FROM languages WHERE code = 'en'),21, 'Satan: Fallen Angel', 'The Quran clearly states that Satan was an angel, by virtue of the immense powers and rank bestowed upon him...'), ((SELECT id FROM languages WHERE code = 'en'),22, 'Jesus', 'The Quran, informs us that Jesus was a human messenger of God whose sole mission was to deliver God''s message; he never possessed any power, and is now dead (4:171, 5:75, 5:117)...'), ((SELECT id FROM languages WHERE code = 'en'),23, 'Chronological Order of Revelation', 'Chronological Sequence of Revelation...'), ((SELECT id FROM languages WHERE code = 'en'),24, 'Two False Verses Removed from the Quran', 'A superhuman mathematical system pervades the Quran and serves to guard and authenticate every element in it...'), ((SELECT id FROM languages WHERE code = 'en'),25, 'End of the World', 'The Quran provides many signs, and states that the means for unveiling the end of the world have been given (47:18)...'), ((SELECT id FROM languages WHERE code = 'en'),26, 'The Three Messengers of Islam', '...it is mathematically coded into the Quran that Abraham, Muhammad, and Rashad are the three messengers of Islam (Submission)...'), ((SELECT id FROM languages WHERE code = 'en'),27, 'Who Is Your God?', '...most of these people will be shocked to find out that their proclamation that their god is the Creator of the heavens and the earth is no more than lip service, and that they are in fact destined for Hell (12:106)...'), ((SELECT id FROM languages WHERE code = 'en'),28, 'Muhammad Wrote God''s Revelations With His Own Hand', 'The Quran tells us that Muhammad wrote down the Quran - Muhammad''s contemporaries are quoted as saying, [Quran 25:5] They also said, ''Tales from the past that he wrote down; they were dictated to him day and night.''...'), ((SELECT id FROM languages WHERE code = 'en'),29, 'The Missing Basmalah', 'This conspicuous absence of the Basmalah from Sura 9 has been an intriguing feature of the Quran for 14 centuries...'), ((SELECT id FROM languages WHERE code = 'en'),30, 'Polygamy', 'Polygamy is permitted in the Quran, but under strictly observed circumstances...'), ((SELECT id FROM languages WHERE code = 'en'),31, 'Evolution: A Divinely Guided Process', 'Humans are not descendants of monkeys...'), ((SELECT id FROM languages WHERE code = 'en'),32, 'The Crucial Age of 40', 'At what age will the human being be held responsible for his or her beliefs?...'), ((SELECT id FROM languages WHERE code = 'en'),33, 'Why Did God Send a Messenger Now?', 'As stated in 3:81, and in Appendix 2, God has sent a messenger to consolidate the messages delivered by all the prophets, purify them, and unify them into one religion: Submission...'), ((SELECT id FROM languages WHERE code = 'en'),34, 'Virginity/Chastity: A Trait of the True Believers', 'Sons and daughters of the believers must be taught that their happiness throughout their lives depends on following God''s law and preserving their chastity...'), ((SELECT id FROM languages WHERE code = 'en'),35, 'Drugs & Alcohol', 'There is no compromise whatsoever regarding illicit drugs and alcoholic beverages; they are called: abominations and the work of Satan (5:90)...'), ((SELECT id FROM languages WHERE code = 'en'),36, 'What Price A Great Nation', 'A nation that upholds God''s laws is guaranteed prominence among the nations of the world, victory, prosperity, and happiness (10:62-64, 16:97, 24:55, 41:30-31)...'), ((SELECT id FROM languages WHERE code = 'en'),37, 'Criminal Justice in Islam', 'The Quran solves this problem, as well as the problems associated with the criminal justice systems prevalent in today''s world...'), ((SELECT id FROM languages WHERE code = 'en'),38, 'The Creator''s Signature', 'This divine stamp appears throughout the universe in much the same manner as the signature of Michelangelo and Picasso identify their works...')
ON CONFLICT (lang_id, code) DO NOTHING;

-----------------------------------------
-- Cluster
-----------------------------------------
CLUSTER verses USING idx_verses_nav;
