-- Legacy source-table corrections.
-- These tables only exist in dev environments after `make source-import`;
-- on Railway / production the data lives in the normalized schema (verses,
-- words, verse_text, word_text). The corresponding production updates are in
-- a follow-up migration. Guard so this is a no-op when the legacy tables are
-- absent.
DO $$
BEGIN
    IF to_regclass('public.ws_quran_word_by_word') IS NOT NULL THEN
        UPDATE ws_quran_word_by_word SET arabic    = 'مُفْسِدِينَ' WHERE index = 908;
        UPDATE ws_quran_word_by_word SET arabic    = 'وَأَنَّهُم'   WHERE index = 34971;
        UPDATE ws_quran_word_by_word SET root_word = 'ك و ن'      WHERE index = 35818;
        UPDATE ws_quran_word_by_word SET arabic    = 'إِلَـٰهِكَ'  WHERE index = 41028;
        UPDATE ws_quran_word_by_word SET arabic    = 'عَلَيْهِمُ'  WHERE index = 41939;
        UPDATE ws_quran_word_by_word SET root_word = 'س و ء'      WHERE index = 48198;
        UPDATE ws_quran_word_by_word SET arabic    = 'ثُمَّ'       WHERE index = 51356;
        UPDATE ws_quran_word_by_word SET arabic    = 'فَبِأَيِّ'   WHERE index = 69029;
    END IF;

    IF to_regclass('public.ws_quran_text') IS NOT NULL THEN
        UPDATE ws_quran_text
        SET transliterated = 'wa-arsala ʿalayhim ṭayran abābīla'
        WHERE verse_id = '105:3';

        UPDATE ws_quran_text
        SET english = replace(english,
            'My Lord, make me one who consistently observes',
            'Our Lord, make me one who consistently observes')
        WHERE verse_id = '14:40';
    END IF;
END $$;
