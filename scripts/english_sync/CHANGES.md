# English translation sync, October 29, 2026

ws-backend's English (`verse_text`, language `en`) compared with the WikiSubmission monorepo's English (`quran_verses`, `quran_subtitles`, `quran_footnotes`, production snapshot of October 28). ws-backend's live API matched its March export exactly, so each change below is what readers of ws-backend saw before the sync.

338 changes in 334 fields across 330 verses, in the categories below. Each change has an id; any category or id can be held back on its own, even when it shares a field with another change. Removed text is struck through, added text underlined.

| | Category | Changes | Recommendation | In this run |
| --- | --- | --- | --- | --- |
| A | Wording | 5 | apply | all |
| B | "Submission" to "Islam" | 9 | apply | all |
| C | Heading placement, 14:29 to 14:31 | 1 | apply | none |
| D | "GOD" to "God" inside brackets | 14 | apply | none |
| E | Unnumbered Basmalah | 112 | your call | none |
| F | Missing footnote marker | 1 | apply | none |
| G | Brackets | 4 | your call | none |
| H | Double quotes around speech | 13 | your call | none |
| I | Ayn mark and nested quotes | 4 | apply | none |
| J | Spacing in the initials | 2 | don't apply | none |
| K | Spaces around nested quotes, dashes and slashes | 9 | your call | none |
| L | Space before a footnote marker | 1 | don't apply | none |
| M | Curly and straight quotes | 2 | split | none |
| N | Stray quotes around a footnote | 1 | apply | none |
| O | Heading footnote marker | 160 | your call | none |

## A. Wording (5)

Changes of meaning. Each verse's new text matches Masjid Tucson's online Quran (masjidtucson.org/quran/noframes) word for word, checked October 29, 2026. Recommendation: apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| A1 | 2:72 content | You had killed a <del>person</del>, then disputed among yourselves. GOD was to expose what you tried to conceal. | You had killed a <ins>soul</ins>, then disputed among yourselves. GOD was to expose what you tried to conceal. |
| A2 | 2:286 content | …ur Lord, protect us from sinning until it becomes too late for us to repent. Pardon us<del>, forgive us,</del> and <del>have mercy on</del> us. You are our Lord and Master. Grant us victory over the disbelieving people." | …ur Lord, protect us from sinning until it becomes too late for us to repent. Pardon us and <ins>forgive</ins> us. You are our Lord and Master. Grant us victory over the disbelieving people." |
| A3 | 14:40 content | "<del>Our</del> Lord, make me one who consistently observes the Contact Prayers (Salat), and also my children. Our Lord, please answer my prayers. | "<ins>My</ins> Lord, make me one who consistently observes the Contact Prayers (Salat), and also my children. Our Lord, please answer my prayers. |
| A4 | 70:4 content | The angels<del> and the Spirit</del>, with their reports, climb to Him in a day that equals fifty thousand years. | The angels, with their reports, climb to Him in a day that equals fifty thousand years. |
| A5 | 70:41 content | to substitute better people in <del>their</del> place; we can never be defeated. | to substitute better people in <ins>your</ins> place; we can never be defeated. |

## B. "Submission" to "Islam" (9)

Terminology in headings and footnotes. 2:127 and 2:135 keep both words. Each new heading and footnote matches Masjid Tucson's online Quran word for word, checked October 29, 2026. Recommendation: apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| B1 | 2:127 subtitle | Abraham Delivered All the Practices of Submission | Abraham Delivered All the Practices of Submission<ins> (Islam)</ins> |
| B2 | 2:135 subtitle | Submission: Abraham's Religion\* | Submission<ins> (Islam)</ins>: Abraham's Religion\* |
| B3 | 4:125 subtitle | Abraham: Original Messenger of <del>Submission</del>\* | Abraham: Original Messenger of <ins>Islam</ins>\* |
| B4 | 21:73 subtitle | Abraham: Delivered All Religious Duties of <del>Submission</del> | Abraham: Delivered All Religious Duties of <ins>Islam</ins> |
| B5 | 21:96 footer | …to God's mathematical miracle in the Quran (Appendix 1), America will be the heart of <del>Submission</del>, and billions around the globe will have believed in the Quran (9:33, 41:53, 48:28, 61:9). Gog and Magog (allegorical names of villainous communities), … | …to God's mathematical miracle in the Quran (Appendix 1), America will be the heart of <ins>Islam</ins>, and billions around the globe will have believed in the Quran (9:33, 41:53, 48:28, 61:9). Gog and Magog (allegorical names of villainous communities), will … |
| B6 | 22:26 subtitle | Pilgrimage, Like All Duties in <del>Submission</del>, Decreed Through Abraham\* | Pilgrimage, Like All Duties in <ins>Islam</ins>, Decreed Through Abraham\* |
| B7 | 22:78 subtitle | Abraham: Original Messenger of <del>Submission</del>\* | Abraham: Original Messenger of <ins>Islam</ins>\* |
| B8 | 39:45 footer | ±39:45 Despite the clear commandment in 3:18 that the First Pillar of <del>Submission</del> is proclaiming: "Ash-hadu An Lã Elãha Ellã Allãh (there is no other god beside God)," the majority of "Muslims" insist upon adding the name of Muhammad. This Greatest C… | ±39:45 Despite the clear commandment in 3:18 that the First Pillar of <ins>Islam</ins> is proclaiming: "Ash-hadu An Lã Elãha Ellã Allãh (there is no other god beside God)," the majority of "Muslims" insist upon adding the name of Muhammad. This Greatest Criter… |
| B9 | 63:1 footer | ±63:1 The "first pillar of <del>Submission</del>," as stated in 3:18 is to bear witness that God is the only god. But the corrupted "Muslim" scholars add "Muhammad is God's messenger," and this violates a number of commandments (see 2:285). Verse 63:1 is the o… | ±63:1 The "first pillar of <ins>Islam</ins>," as stated in 3:18 is to bear witness that God is the only god. But the corrupted "Muslim" scholars add "Muhammad is God's messenger," and this violates a number of commandments (see 2:285). Verse 63:1 is the only p… |

## C. Heading placement, 14:29 to 14:31 (1)

ws-backend set both headings one verse early. "Idol Worship" belongs on 14:30 and "Crucial Commandments" on 14:31, the verses they introduce. Recommendation: apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| C1 (held) | 14:29 subtitle | <del>Idol Worship: The Mother of All Evil</del> | (empty) |
| C1 (held) | 14:30 subtitle | <del>Crucial</del> <del>Commandments</del> | <ins>Idol</ins> <ins>Worship: The Mother of All Evil</ins> |
| C1 (held) | 14:31 subtitle | (empty) | <ins>Crucial Commandments</ins> |

## D. "GOD" to "God" inside brackets (14)

Capital "GOD" marks the Arabic word Allah. Bracketed words are not in the Arabic, and the monorepo has no capital GOD inside brackets anywhere. Recommendation: apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| D1 (held) | 2:124 content | …am was put to the test by his Lord, through certain commands, and he fulfilled them. (<del>GOD</del>) said, "I am appointing you an imam for the people." He said, "And also my descendants?" He said, "My covenant does not include the transgressors." | …am was put to the test by his Lord, through certain commands, and he fulfilled them. (<ins>God</ins>) said, "I am appointing you an imam for the people." He said, "And also my descendants?" He said, "My covenant does not include the transgressors." |
| D2 (held) | 2:126 content | …vide its people with fruits. Provide for those who believe in GOD and the Last Day." (<del>GOD</del>) said, "I will also provide for those who disbelieve. I will let them enjoy, temporarily, then commit them to the retribution of Hell, and a miserable destiny.… | …vide its people with fruits. Provide for those who believe in GOD and the Last Day." (<ins>God</ins>) said, "I will also provide for those who disbelieve. I will let them enjoy, temporarily, then commit them to the retribution of Hell, and a miserable destiny.… |
| D3 (held) | 9:98 content | Some Arabs consider their spending (in the cause of <del>GOD</del>) to be a loss, and even wait in anticipation that a disaster may hit you. It is they who will incur the worst disaster. GOD is Hearer, Omniscient. | Some Arabs consider their spending (in the cause of <ins>God</ins>) to be a loss, and even wait in anticipation that a disaster may hit you. It is they who will incur the worst disaster. GOD is Hearer, Omniscient. |
| D4 (held) | 16:91 content | You shall fulfill your covenant with GOD when you make such a covenant. You shall not violate the oaths after swearing (by <del>GOD</del>) to carry them out, for you have made GOD a guarantor for you. GOD knows everything you do. | You shall fulfill your covenant with GOD when you make such a covenant. You shall not violate the oaths after swearing (by <ins>God</ins>) to carry them out, for you have made GOD a guarantor for you. GOD knows everything you do. |
| D5 (held) | 19:11 content | He came out to his family, from the sanctuary, and signaled to them: "Meditate (on <del>GOD</del>) day and night." | He came out to his family, from the sanctuary, and signaled to them: "Meditate (on <ins>God</ins>) day and night." |
| D6 (held) | 21:79 content | We granted Solomon the correct understanding, though we endowed both of them with wisdom and knowledge. We committed the mountains to serve David in glorifying (<del>GOD</del>), as well as the birds. This is what we did. | We granted Solomon the correct understanding, though we endowed both of them with wisdom and knowledge. We committed the mountains to serve David in glorifying (<ins>God</ins>), as well as the birds. This is what we did. |
| D7 (held) | 24:36 content | (<del>GOD</del>'s guidance is found) in houses exalted by GOD, for His name is commemorated therein. Glorifying Him therein, day and night— | (<ins>God</ins>'s guidance is found) in houses exalted by GOD, for His name is commemorated therein. Glorifying Him therein, day and night— |
| D8 (held) | 27:75 content | There is nothing in the heavens and the earth that is hidden (from <del>GOD</del>); everything is in a profound record. | There is nothing in the heavens and the earth that is hidden (from <ins>God</ins>); everything is in a profound record. |
| D9 (held) | 34:39 content | Say, "My Lord is the One who controls all provisions; He increases the provisions for whomever He chooses from among His servants, or reduces them. Anything you spend (in the cause of <del>GOD</del>), He will reward you for it; He is the Best Provider." | Say, "My Lord is the One who controls all provisions; He increases the provisions for whomever He chooses from among His servants, or reduces them. Anything you spend (in the cause of <ins>God</ins>), He will reward you for it; He is the Best Provider." |
| D10 (held) | 36:14 content | When we sent to them two (messengers), they disbelieved them. We then supported them by a third. They said, "We are (<del>GOD</del>'s) messengers to you." | When we sent to them two (messengers), they disbelieved them. We then supported them by a third. They said, "We are (<ins>God</ins>'s) messengers to you." |
| D11 (held) | 37:143 content | If it were not that he resorted to meditation (on <del>GOD</del>), | If it were not that he resorted to meditation (on <ins>God</ins>), |
| D12 (held) | 44:23 content | (<del>GOD</del> said,) "Travel with My servants during the night; you will be pursued. | (<ins>God</ins> said,) "Travel with My servants during the night; you will be pursued. |
| D13 (held) | 56:11 content | They are those who will be closest (to <del>GOD</del>). | They are those who will be closest (to <ins>God</ins>). |
| D14 (held) | 68:28 content | The righteous among them said, "If only you had glorified (<del>GOD</del>)!" | The righteous among them said, "If only you had glorified (<ins>God</ins>)!" |

## E. Unnumbered Basmalah (112)

The Basmalah that opens suras 2 to 114, stored as verse 0. The monorepo writes "God" and drops the final period. Lower case may be deliberate, since unnumbered Basmalahs fall outside the count of GOD, but nothing in the repository says so. 16:0 alone also capitalizes "Name". Masjid Tucson's sura pages write it the monorepo's way (checked on 63:0). Recommendation: your call.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| E1 (held) | 2:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E2 (held) | 3:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E3 (held) | 4:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E4 (held) | 5:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E5 (held) | 6:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E6 (held) | 7:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E7 (held) | 8:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E8 (held) | 10:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E9 (held) | 11:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E10 (held) | 12:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E11 (held) | 13:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E12 (held) | 14:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E13 (held) | 15:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E14 (held) | 16:0 content | In the <del>name</del> of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the <ins>Name</ins> of <ins>God</ins>, Most Gracious, Most Merciful |
| E15 (held) | 17:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E16 (held) | 18:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E17 (held) | 19:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E18 (held) | 20:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E19 (held) | 21:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E20 (held) | 22:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E21 (held) | 23:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E22 (held) | 24:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E23 (held) | 25:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E24 (held) | 26:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E25 (held) | 27:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E26 (held) | 28:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E27 (held) | 29:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E28 (held) | 30:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E29 (held) | 31:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E30 (held) | 32:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E31 (held) | 33:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E32 (held) | 34:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E33 (held) | 35:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E34 (held) | 36:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E35 (held) | 37:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E36 (held) | 38:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E37 (held) | 39:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E38 (held) | 40:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E39 (held) | 41:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E40 (held) | 42:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E41 (held) | 43:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E42 (held) | 44:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E43 (held) | 45:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E44 (held) | 46:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E45 (held) | 47:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E46 (held) | 48:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E47 (held) | 49:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E48 (held) | 50:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E49 (held) | 51:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E50 (held) | 52:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E51 (held) | 53:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E52 (held) | 54:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E53 (held) | 55:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E54 (held) | 56:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E55 (held) | 57:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E56 (held) | 58:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E57 (held) | 59:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E58 (held) | 60:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E59 (held) | 61:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E60 (held) | 62:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E61 (held) | 63:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E62 (held) | 64:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E63 (held) | 65:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E64 (held) | 66:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E65 (held) | 67:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E66 (held) | 68:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E67 (held) | 69:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E68 (held) | 70:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E69 (held) | 71:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E70 (held) | 72:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E71 (held) | 73:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E72 (held) | 74:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E73 (held) | 75:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E74 (held) | 76:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E75 (held) | 77:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E76 (held) | 78:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E77 (held) | 79:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E78 (held) | 80:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E79 (held) | 81:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E80 (held) | 82:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E81 (held) | 83:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E82 (held) | 84:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E83 (held) | 85:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E84 (held) | 86:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E85 (held) | 87:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E86 (held) | 88:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E87 (held) | 89:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E88 (held) | 90:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E89 (held) | 91:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E90 (held) | 92:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E91 (held) | 93:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E92 (held) | 94:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E93 (held) | 95:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E94 (held) | 96:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E95 (held) | 97:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E96 (held) | 98:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E97 (held) | 99:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E98 (held) | 100:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E99 (held) | 101:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E100 (held) | 102:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E101 (held) | 103:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E102 (held) | 104:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E103 (held) | 105:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E104 (held) | 106:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E105 (held) | 107:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E106 (held) | 108:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E107 (held) | 109:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E108 (held) | 110:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E109 (held) | 111:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E110 (held) | 112:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E111 (held) | 113:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |
| E112 (held) | 114:0 content | In the name of <del>GOD</del>, Most Gracious, Most Merciful<del>.</del> | In the name of <ins>God</ins>, Most Gracious, Most Merciful |

## F. Missing footnote marker (1)

2:142 has a footnote in both texts, but ws-backend's verse lacked the marker. Recommendation: apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| F1 (held) | 2:142 content | The fools among the people would say, "Why did they change the direction of their Qiblah?" Say, "To GOD belongs the east and the west; He guides whoever wills in a straight path." | The fools among the people would say, "Why did they change the direction of their Qiblah?"<ins>±</ins> Say, "To GOD belongs the east and the west; He guides whoever wills in a straight path." |

## G. Brackets (4)

Round brackets become square ones (18:25, 37:35, 110:0), and 29:53 drops them. Recommendation: your call.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| G1 (held) | 18:25 subtitle | <del>(</del>300 + 9<del>)</del>\* | <ins>[</ins>300 + 9<ins>]</ins>\* |
| G2 (held) | 29:53 subtitle | They <del>(</del>Are<del>)</del> in Hell | They Are in Hell |
| G3 (held) | 37:35 content | When they were told, "Lã Elãha Ella Allãh <del>(</del>There is no other god beside GOD<del>)</del>," they turned arrogant. | When they were told, "Lã Elãha Ella Allãh <ins>[</ins>There is no other god beside GOD<ins>]</ins>," they turned arrogant. |
| G4 (held) | 110:0 subtitle | <del>(</del>Last Sura Revealed<del>)</del>\* | <ins>[</ins>Last Sura Revealed<ins>]</ins>\* |

## H. Double quotes around speech (13)

Quotes opened, closed or left open where a speech starts in one verse and runs into the next. Recommendation: your call.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| H1 (held) | 2:39 content | As for those who disbelieve and reject our revelations, they will be dwellers of Hell, wherein they abide forever. | <ins>"</ins>As for those who disbelieve and reject our revelations, they will be dwellers of Hell, wherein they abide forever.<ins>"</ins> |
| H2 (held) | 3:48 content | He will teach him the scripture, wisdom, the Torah, and the Gospel. | <ins>"</ins>He will teach him the scripture, wisdom, the Torah, and the Gospel.<ins>"</ins> |
| H3 (held) | 5:84 content | Why should we not believe in GOD, and in the truth that has come to us, and hope that our Lord may admit us with the righteous people? | <ins>"</ins>Why should we not believe in GOD, and in the truth that has come to us, and hope that our Lord may admit us with the righteous people?<ins>"</ins> |
| H4 (held) | 7:88 content | The arrogant leaders among his people said, "We will evict you, O Shu'aib, together with those who believed with you, from our town, unless you revert to our religion." He said, "Are you going to force us?<del>"</del> | The arrogant leaders among his people said, "We will evict you, O Shu'aib, together with those who believed with you, from our town, unless you revert to our religion." He said, "Are you going to force us? |
| H5 (held) | 7:156 content | …ree for us righteousness in this world, and in the Hereafter. We have repented to You. He said, "My retribution befalls whomever I will. But My mercy encompasses all things. However, I will specify it for those who (1) lead a righteous life, (2) give the oblig… | …ree for us righteousness in this world, and in the Hereafter. We have repented to You.<ins>"</ins> He said, "My retribution befalls whomever I will. But My mercy encompasses all things. However, I will specify it for those who (1) lead a righteous life, (2) gi… |
| H6 (held) | 10:29 content | GOD suffices as a witness between us and you, that we were completely unaware of your worshiping us. | <ins>"</ins>GOD suffices as a witness between us and you, that we were completely unaware of your worshiping us.<ins>"</ins> |
| H7 (held) | 12:87 content | "O my sons, go fetch Joseph and his brother, and never despair of GOD's grace. None despairs of GOD's grace except the disbelieving people. | "O my sons, go fetch Joseph and his brother, and never despair of GOD's grace. None despairs of GOD's grace except the disbelieving people.<ins>"</ins> |
| H8 (held) | 28:31 content | Throw down your staff. When he saw it moving like a demon, he turned around and fled. "O Moses, come back; do not be afraid. You are perfectly safe. | <ins>"</ins>Throw down your staff.<ins>"</ins> When he saw it moving like a demon, he turned around and fled. "O Moses, come back; do not be afraid. You are perfectly safe. |
| H9 (held) | 28:32 content | Put your hand into your pocket; it will come out white without a blemish. Fold your wings and settle down from your fear. These are two proofs from your Lord, to be shown to Pharaoh and his elders; they have been wicked people. | <ins>"</ins>Put your hand into your pocket; it will come out white without a blemish. Fold your wings and settle down from your fear. These are two proofs from your Lord, to be shown to Pharaoh and his elders; they have been wicked people.<ins>"</ins> |
| H10 (held) | 28:34 content | Also, my brother Aaron is more eloquent than I. Send him with me as a helper to confirm and strengthen me. I fear lest they disbelieve me. | <ins>"</ins>Also, my brother Aaron is more eloquent than I. Send him with me as a helper to confirm and strengthen me. I fear lest they disbelieve me.<ins>"</ins> |
| H11 (held) | 28:77 content | Use the provisions bestowed upon you by GOD to seek the abode of the Hereafter, without neglecting your share in this world. Be charitable, as GOD has been charitable towards you. Do not keep on corrupting the earth. GOD does not love the corruptors. | <ins>"</ins>Use the provisions bestowed upon you by GOD to seek the abode of the Hereafter, without neglecting your share in this world. Be charitable, as GOD has been charitable towards you. Do not keep on corrupting the earth. GOD does not love the corruptor… |
| H12 (held) | 38:85 content | I will fill Hell with you and all those who follow you. | <ins>"</ins>I will fill Hell with you and all those who follow you.<ins>"</ins> |
| H13 (held) | 46:31 content | O our people, respond to the call of GOD, and believe in Him. He will then forgive your sins, and spare you a painful retribution. | <ins>"</ins>O our people, respond to the call of GOD, and believe in Him. He will then forgive your sins, and spare you a painful retribution.<ins>"</ins> |

## I. Ayn mark and nested quotes (4)

An apostrophe for the ayn ("'Ãd", "'A. S. Q."), used in all 23 of the monorepo's 'Ãd verses, and the opening single quote of a nested speech in 12:82. Recommendation: apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| I1 (held) | 12:82 content | You may ask the community where we were, and the caravan that came back with us. We are telling the truth.'" | <ins>'</ins>You may ask the community where we were, and the caravan that came back with us. We are telling the truth.'" |
| I2 (held) | 26:123 content | Ãd disbelieved the messengers. | <ins>'</ins>Ãd disbelieved the messengers. |
| I3 (held) | 42:2 content | A. S. Q.± | <ins>'</ins>A. S. Q.± |
| I4 (held) | 54:18 content | Ãd disbelieved. Consequently, how terrible was My retribution after the warnings. | <ins>'</ins>Ãd disbelieved. Consequently, how terrible was My retribution after the warnings. |

## J. Spacing in the initials (2)

2:1 and 3:1 become "A.L.M.", but the monorepo spaces the same initials in 29:1 to 32:1 ("A. L. M.") and every other sura but 42:1. Recommendation: don't apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| J1 (held) | 2:1 content | A.<del>[space]</del>L.<del>[space]</del>M.± | A.L.M.± |
| J2 (held) | 3:1 content | A.<del>[space]</del>L.<del>[space]</del>M.± | A.L.M.± |

## K. Spaces around nested quotes, dashes and slashes (9)

Cosmetic. The monorepo is not consistent here: 3 verses keep the space between quote marks and 13 keep a space after a dash. Recommendation: your call.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| K1 (held) | 5:67 content | O you messenger, deliver what is revealed to you from your Lord—<del>[space]</del>until you do, you have not delivered His message—and GOD will protect you from the people. GOD does not guide the disbelieving people. | O you messenger, deliver what is revealed to you from your Lord—until you do, you have not delivered His message—and GOD will protect you from the people. GOD does not guide the disbelieving people. |
| K2 (held) | 19:26 content | "Eat and drink, and be happy. When you see anyone, say, 'I have made a vow of silence; I am not talking today to anyone.'<del>[space]</del>" | "Eat and drink, and be happy. When you see anyone, say, 'I have made a vow of silence; I am not talking today to anyone.'" |
| K3 (held) | 20:48 content | "<del>[space]</del>'We have been inspired that the retribution will inevitably afflict those who disbelieve and turn away.'<del>[space]</del>" | "'We have been inspired that the retribution will inevitably afflict those who disbelieve and turn away.'" |
| K4 (held) | 21:25 subtitle | One God<del>[space]</del>/ One Message<del>[space]</del>/ One Religion | One God/ One Message/ One Religion |
| K5 (held) | 23:29 content | "And say, 'My Lord, let me disembark onto a blessed location; You are the best deliverer.'<del>[space]</del>" | "And say, 'My Lord, let me disembark onto a blessed location; You are the best deliverer.'" |
| K6 (held) | 26:17 content | "<del>[space]</del>'Let the Children of Israel go.'<del>[space]</del>" | "'Let the Children of Israel go.'" |
| K7 (held) | 27:31 content | "Proclaiming: 'Do not be arrogant; come to me as submitters.'<del>[space]</del>" | "Proclaiming: 'Do not be arrogant; come to me as submitters.'" |
| K8 (held) | 37:53 content | "<del>[space]</del>'After we die and turn into dust and bones, do we get called to account?'<del>[space]</del>" | "'After we die and turn into dust and bones, do we get called to account?'" |
| K9 (held) | 71:12 content | "<del>[space]</del>'And provide you with money and children, and orchards, and streams.'<del>[space]</del>" | "'And provide you with money and children, and orchards, and streams.'" |

## L. Space before a footnote marker (1)

11:17 gains a space before ±, as in 7 other monorepo verses; most of the text has none. Recommendation: don't apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| L1 (held) | 11:17 content | As for those who are given solid proof± from their Lord, reported by a witness from Him, and before it, the book of Moses has set a precedent and a mercy,±± they will surely believe. As for those who disbelieve among the various groups, Hell is awaiting them. … | As for those who are given solid proof<ins>[space]</ins>± from their Lord, reported by a witness from Him, and before it, the book of Moses has set a precedent and a mercy,±± they will surely believe. As for those who disbelieve among the various groups, Hell … |

## M. Curly and straight quotes (2)

The text uses straight quotes. 6:50 drops a stray curly one (apply); the 5:116 footnote turns its quotes curly, the only footnote that would have them (don't apply). Recommendation: split.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| M1 (held) | 6:50 content | Say, <del>“</del>I do not say to you that I possess the treasures of GOD. Nor do I know the future. Nor do I say to you that I am an angel. I simply follow what is revealed to me." Say, "Is the blind the same as the seer? Do you not reflect?" | Say, <ins>"</ins>I do not say to you that I possess the treasures of GOD. Nor do I know the future. Nor do I say to you that I am an angel. I simply follow what is revealed to me." Say, "Is the blind the same as the seer? Do you not reflect?" |
| M2 (held) | 5:116 footer | ±5:116 It is noteworthy that the Quran consistently calls Jesus <del>"</del>son of Mary<del>"</del> and the Bible calls him <del>"</del>son of man.<del>"</del> God knew that some will blaspheme and call him <del>"</del>son of God<del>"</del>! | ±5:116 It is noteworthy that the Quran consistently calls Jesus <ins>“</ins>son of Mary<ins>”</ins> and the Bible calls him <ins>“</ins>son of man.<ins>”</ins> God knew that some will blaspheme and call him <ins>“</ins>son of God<ins>”</ins>! |

## N. Stray quotes around a footnote (1)

The 6:113 footnote is wrapped in quotation marks in ws-backend. Recommendation: apply.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| N1 (held) | 6:113 footer | <del>"</del>±6:113 The Quran provides criteria that tell us whether we truly believe in the Hereafter or merely give it lip service. These important criteria are stated here and in 17:45-46 and 39:45.  ±6:113-115 Upholding any source beside the Quran reflects … | ±6:113 The Quran provides criteria that tell us whether we truly believe in the Hereafter or merely give it lip service. These important criteria are stated here and in 17:45-46 and 39:45.  ±6:113-115 Upholding any source beside the Quran reflects disbelief in… |

## O. Heading footnote marker (160)

Headings that have a footnote end in * in ws-backend and ± in the monorepo, the marker verse text already uses. Readers see the character itself. Masjid Tucson's headings use *. Recommendation: your call.

| Id | Verse | ws-backend before | Monorepo (after) |
| --- | --- | --- | --- |
| O1 (held) | 2:28 subtitle | Two Deaths and Two Lives for the Disbelievers<del>\*</del> | Two Deaths and Two Lives for the Disbelievers<ins>±</ins> |
| O2 (held) | 2:30 subtitle | Satan: A Temporary "god"<del>\*</del> | Satan: A Temporary "god"<ins>±</ins> |
| O3 (held) | 2:37 subtitle | Specific Words<del>\*</del> | Specific Words<ins>±</ins> |
| O4 (held) | 2:54 subtitle | Kill Your Ego<del>\*</del> | Kill Your Ego<ins>±</ins> |
| O5 (held) | 2:55 subtitle | Physical Evidence<del>\*</del> | Physical Evidence<ins>±</ins> |
| O6 (held) | 2:67 subtitle | The Heifer<del>\*</del> | The Heifer<ins>±</ins> |
| O7 (held) | 2:80 subtitle | Eternity of Heaven and Hell<del>\*</del> | Eternity of Heaven and Hell<ins>±</ins> |
| O8 (held) | 2:106 subtitle | The Ultimate Miracle: The Quran's Mathematical Code<del>\*</del> | The Ultimate Miracle: The Quran's Mathematical Code<ins>±</ins> |
| O9 (held) | 2:135 subtitle | Submission: Abraham's Religion<del>\*</del> | Submission: Abraham's Religion<ins>±</ins> |
| O10 (held) | 2:142 subtitle | Abolition of Bigotry and Prejudice<del>\*</del> | Abolition of Bigotry and Prejudice<ins>±</ins> |
| O11 (held) | 2:165 subtitle | The Idols Disown Their Idolizers<del>\*</del> | The Idols Disown Their Idolizers<ins>±</ins> |
| O12 (held) | 2:172 subtitle | Only Four Meats Prohibited<del>\*</del> | Only Four Meats Prohibited<ins>±</ins> |
| O13 (held) | 2:174 subtitle | Corrupted Religious Leaders Conceal the Quran's Miracle<del>\*</del> | Corrupted Religious Leaders Conceal the Quran's Miracle<ins>±</ins> |
| O14 (held) | 2:178 subtitle | Discouraging Capital Punishment<del>\*</del> | Discouraging Capital Punishment<ins>±</ins> |
| O15 (held) | 2:183 subtitle | Fasting Emphasized and Modified<del>\*</del> | Fasting Emphasized and Modified<ins>±</ins> |
| O16 (held) | 2:190 subtitle | Rules of War<del>\*</del> | Rules of War<ins>±</ins> |
| O17 (held) | 2:196 subtitle | Hajj and 'Umrah Pilgrimage<del>\*</del> | Hajj and 'Umrah Pilgrimage<ins>±</ins> |
| O18 (held) | 2:211 subtitle | Miracles Bring Greater Responsibility<del>\*</del> | Miracles Bring Greater Responsibility<ins>±</ins> |
| O19 (held) | 2:219 subtitle | Intoxicants and Gambling Prohibited<del>\*</del> | Intoxicants and Gambling Prohibited<ins>±</ins> |
| O20 (held) | 2:238 subtitle | You Shall Observe the Contact Prayers<del>\*</del> | You Shall Observe the Contact Prayers<ins>±</ins> |
| O21 (held) | 2:246 subtitle | Saul<del>\*</del> | Saul<ins>±</ins> |
| O22 (held) | 2:254 subtitle | No Intercession<del>\*</del> | No Intercession<ins>±</ins> |
| O23 (held) | 2:259 subtitle | Lesson About Death<del>\*</del> | Lesson About Death<ins>±</ins> |
| O24 (held) | 2:275 subtitle | Usury Prohibited<del>\*</del> | Usury Prohibited<ins>±</ins> |
| O25 (held) | 3:18 subtitle | The Most Important Commandment<del>\*</del> | The Most Important Commandment<ins>±</ins> |
| O26 (held) | 3:54 subtitle | The Death of Jesus<del>\*</del> | The Death of Jesus<ins>±</ins> |
| O27 (held) | 3:59 subtitle | Mathematical Confirmation<del>\*</del> | Mathematical Confirmation<ins>±</ins> |
| O28 (held) | 3:81 subtitle | Major Prophecy Fulfilled<del>\*</del> God's Messenger of the Covenant | Major Prophecy Fulfilled<ins>±</ins> God's Messenger of the Covenant |
| O29 (held) | 3:130 subtitle | Usury Prohibited<del>\*</del> | Usury Prohibited<ins>±</ins> |
| O30 (held) | 3:169 subtitle | The Righteous Do Not Really Die<del>\*</del> | The Righteous Do Not Really Die<ins>±</ins> |
| O31 (held) | 3:186 subtitle | The Inevitable Test<del>\*</del> | The Inevitable Test<ins>±</ins> |
| O32 (held) | 4:3 subtitle | Grounds For Polygamy<del>\*</del> | Grounds For Polygamy<ins>±</ins> |
| O33 (held) | 4:11 subtitle | If No Will Is Left<del>\*</del> | If No Will Is Left<ins>±</ins> |
| O34 (held) | 4:34 subtitle | Do Not Beat Your Wife<del>\*</del> | Do Not Beat Your Wife<ins>±</ins> |
| O35 (held) | 4:78 subtitle | God is the Doer of Everything<del>\*</del> | God is the Doer of Everything<ins>±</ins> |
| O36 (held) | 4:125 subtitle | Abraham: Original Messenger of Submission<del>\*</del> | Abraham: Original Messenger of Submission<ins>±</ins> |
| O37 (held) | 4:129 subtitle | Polygamy Discouraged<del>\*</del> | Polygamy Discouraged<ins>±</ins> |
| O38 (held) | 4:157 subtitle | Crucifying "the Body" of Jesus<del>\*</del> | Crucifying "the Body" of Jesus<ins>±</ins> |
| O39 (held) | 5:12 subtitle | Conditions for Staying Within God's Protection<del>\*</del> | Conditions for Staying Within God's Protection<ins>±</ins> |
| O40 (held) | 5:27 subtitle | The First Murder<del>\*</del> | The First Murder<ins>±</ins> |
| O41 (held) | 5:51 subtitle | Certain Jews and Christians Cannot Be Friends<del>\*</del> | Certain Jews and Christians Cannot Be Friends<ins>±</ins> |
| O42 (held) | 5:72 subtitle | Today's Christianity Not Jesus' Religion<del>\*</del> | Today's Christianity Not Jesus' Religion<ins>±</ins> |
| O43 (held) | 5:114 subtitle | Greater Miracles Bring Greater Responsibility<del>\*</del> | Greater Miracles Bring Greater Responsibility<ins>±</ins> |
| O44 (held) | 6:38 subtitle | Animals and Birds: Submitting Creatures<del>\*</del> | Animals and Birds: Submitting Creatures<ins>±</ins> |
| O45 (held) | 6:44 subtitle | The System<del>\*</del> | The System<ins>±</ins> |
| O46 (held) | 6:60 subtitle | Death and Resurrection: Every Day<del>\*</del> | Death and Resurrection: Every Day<ins>±</ins> |
| O47 (held) | 6:114 subtitle | Quran: Fully Detailed<del>\*</del> | Quran: Fully Detailed<ins>±</ins> |
| O48 (held) | 6:124 subtitle | Questioning God's Wisdom<del>\*</del> | Questioning God's Wisdom<ins>±</ins> |
| O49 (held) | 6:125 subtitle | Quranic Knowledge Far Ahead of Human Progress<del>\*</del> | Quranic Knowledge Far Ahead of Human Progress<ins>±</ins> |
| O50 (held) | 6:141 subtitle | Zakat Must Be Given "On The Day Of Harvest"<del>\*</del> | Zakat Must Be Given "On The Day Of Harvest"<ins>±</ins> |
| O51 (held) | 6:145 subtitle | The Only Dietary Prohibitions<del>\*</del> | The Only Dietary Prohibitions<ins>±</ins> |
| O52 (held) | 6:149 subtitle | The Most Powerful Argument<del>\*</del> | The Most Powerful Argument<ins>±</ins> |
| O53 (held) | 6:157 subtitle | Mathematics: The Ultimate Proof<del>\*</del> | Mathematics: The Ultimate Proof<ins>±</ins> |
| O54 (held) | 7:172 subtitle | We Are Born With Instinctive Knowledge About God<del>\*</del> | We Are Born With Instinctive Knowledge About God<ins>±</ins> |
| O55 (held) | 8:12 subtitle | Lessons from History<del>\*</del> | Lessons from History<ins>±</ins> |
| O56 (held) | 8:17 subtitle | God Is Doing Everything<del>\*</del> | God Is Doing Everything<ins>±</ins> |
| O57 (held) | 8:24 subtitle | The Righteous Do Not Really Die<del>\*</del> | The Righteous Do Not Really Die<ins>±</ins> |
| O58 (held) | 8:30 subtitle | God Protects His Messenger<del>\*</del> | God Protects His Messenger<ins>±</ins> |
| O59 (held) | 8:35 subtitle | The Contact Prayers (Salat) Existed Before the Quran<del>\*</del> | The Contact Prayers (Salat) Existed Before the Quran<ins>±</ins> |
| O60 (held) | 8:36 subtitle | Spending Their Money to Fight God<del>\*</del> | Spending Their Money to Fight God<ins>±</ins> |
| O61 (held) | 9:1 subtitle | No Basmalah<del>\*</del> | No Basmalah<ins>±</ins> |
| O62 (held) | 9:24 subtitle | Important Criterion<del>\*</del> | Important Criterion<ins>±</ins> |
| O63 (held) | 9:33 subtitle | "Submission" Destined to Prevail<del>\*</del> | "Submission" Destined to Prevail<ins>±</ins> |
| O64 (held) | 9:36 subtitle | God's System: Twelve Months Per Year<del>\*</del> | God's System: Twelve Months Per Year<ins>±</ins> |
| O65 (held) | 9:37 subtitle | Altering the Sacred Months<del>\*</del> | Altering the Sacred Months<ins>±</ins> |
| O66 (held) | 9:54 subtitle | The Contact Prayer Existed Before Muhammad<del>\*</del> | The Contact Prayer Existed Before Muhammad<ins>±</ins> |
| O67 (held) | 9:80 subtitle | Satan's Most Effective Bait: The Myth of Intercession<del>\*</del> | Satan's Most Effective Bait: The Myth of Intercession<ins>±</ins> |
| O68 (held) | 9:101 subtitle | Retribution Doubled for the Hypocrites<del>\*</del> | Retribution Doubled for the Hypocrites<ins>±</ins> |
| O69 (held) | 9:107 subtitle | Masjids that Oppose God and His Messenger<del>\*</del> | Masjids that Oppose God and His Messenger<ins>±</ins> |
| O70 (held) | 9:127 subtitle | A Historical Crime Unveiled: Tampering With the Word of God.<del>\*</del> God Provides Irrefutable Evidence | A Historical Crime Unveiled: Tampering With the Word of God.<ins>±</ins> God Provides Irrefutable Evidence |
| O71 (held) | 10:20 subtitle | Miracle of the Quran to be Unveiled After Muhammad<del>\*</del> | Miracle of the Quran to be Unveiled After Muhammad<ins>±</ins> |
| O72 (held) | 10:62 subtitle | Happiness: Now and Forever<del>\*</del> | Happiness: Now and Forever<ins>±</ins> |
| O73 (held) | 10:92 subtitle | Pharaoh's Body Preserved<del>\*</del> | Pharaoh's Body Preserved<ins>±</ins> |
| O74 (held) | 10:100 subtitle | The Disbelievers Blocked Out<del>\*</del> | The Disbelievers Blocked Out<ins>±</ins> |
| O75 (held) | 11:46 subtitle | The Myth of Intercession<del>\*</del> | The Myth of Intercession<ins>±</ins> |
| O76 (held) | 12:9 subtitle | Joseph's Fate Already Decided by God<del>\*</del> | Joseph's Fate Already Decided by God<ins>±</ins> |
| O77 (held) | 13:30 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O78 (held) | 13:37 subtitle | Divine Authorization of the Quran's Mathematical Code<del>\*</del> | Divine Authorization of the Quran's Mathematical Code<ins>±</ins> |
| O79 (held) | 14:48 subtitle | New Heavens and New Earth<del>\*</del> | New Heavens and New Earth<ins>±</ins> |
| O80 (held) | 15:9 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O81 (held) | 15:85 subtitle | End of the World Unveiled<del>\*</del> | End of the World Unveiled<ins>±</ins> |
| O82 (held) | 16:30 subtitle | The Believers Do Not Really Die<del>\*</del> | The Believers Do Not Really Die<ins>±</ins> |
| O83 (held) | 16:98 subtitle | An Important Commandment<del>\*</del> | An Important Commandment<ins>±</ins> |
| O84 (held) | 16:123 subtitle | Muhammad: A Follower of Abraham<del>\*</del> | Muhammad: A Follower of Abraham<ins>±</ins> |
| O85 (held) | 17:13 subtitle | The Video Tape<del>\*</del> | The Video Tape<ins>±</ins> |
| O86 (held) | 18:7 subtitle | The End of the World<del>\*</del> | The End of the World<ins>±</ins> |
| O87 (held) | 18:16 subtitle | The Seven Sleepers of Ephesus<del>\*</del> | The Seven Sleepers of Ephesus<ins>±</ins> |
| O88 (held) | 18:21 subtitle | Connection With the End of the World<del>\*</del> | Connection With the End of the World<ins>±</ins> |
| O89 (held) | 18:25 subtitle | (300 + 9)<del>\*</del> | (300 + 9)<ins>±</ins> |
| O90 (held) | 18:32 subtitle | Property as an Idol<del>\*</del> | Property as an Idol<ins>±</ins> |
| O91 (held) | 18:94 subtitle | Gog and Magog<del>\*</del> | Gog and Magog<ins>±</ins> |
| O92 (held) | 19:71 subtitle | Everyone Sees Hell<del>\*</del> | Everyone Sees Hell<ins>±</ins> |
| O93 (held) | 20:15 subtitle | End of the World is Not Hidden<del>\*</del> | End of the World is Not Hidden<ins>±</ins> |
| O94 (held) | 20:115 subtitle | Humans Fail to Make a Firm Stand<del>\*</del> | Humans Fail to Make a Firm Stand<ins>±</ins> |
| O95 (held) | 21:30 subtitle | The Big Bang Theory Confirmed<del>\*</del> | The Big Bang Theory Confirmed<ins>±</ins> |
| O96 (held) | 21:96 subtitle | The End of the World<del>\*</del> | The End of the World<ins>±</ins> |
| O97 (held) | 22:19 subtitle | How Terrible is Hell!<del>\*</del> | How Terrible is Hell!<ins>±</ins> |
| O98 (held) | 22:26 subtitle | Pilgrimage, Like All Duties in Submission, Decreed Through Abraham<del>\*</del> | Pilgrimage, Like All Duties in Submission, Decreed Through Abraham<ins>±</ins> |
| O99 (held) | 22:33 subtitle | Livestock Offerings During Pilgrimage<del>\*</del> | Livestock Offerings During Pilgrimage<ins>±</ins> |
| O100 (held) | 22:52 subtitle | The System<del>\*</del> | The System<ins>±</ins> |
| O101 (held) | 22:78 subtitle | Abraham: Original Messenger of Submission<del>\*</del> | Abraham: Original Messenger of Submission<ins>±</ins> |
| O102 (held) | 23:84 subtitle | Most Believers are Destined for Hell<del>\*</del> | Most Believers are Destined for Hell<ins>±</ins> |
| O103 (held) | 24:30 subtitle | Dress Code for Believers<del>\*</del> | Dress Code for Believers<ins>±</ins> |
| O104 (held) | 25:27 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O105 (held) | 26:210 subtitle | False Messengers Incapable of Preaching the Worship of God ALONE<del>\*</del> | False Messengers Incapable of Preaching the Worship of God ALONE<ins>±</ins> |
| O106 (held) | 26:214 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O107 (held) | 27:82 subtitle | The Computer Is The Creature<del>\*</del> | The Computer Is The Creature<ins>±</ins> |
| O108 (held) | 28:43 subtitle | The Book of Moses<del>\*</del> | The Book of Moses<ins>±</ins> |
| O109 (held) | 29:19 subtitle | Study the Origin of Life<del>\*</del> | Study the Origin of Life<ins>±</ins> |
| O110 (held) | 29:48 subtitle | Quran: Muhammad's Miracle<del>\*</del> | Quran: Muhammad's Miracle<ins>±</ins> |
| O111 (held) | 30:30 subtitle | Monotheism: Natural Instinct<del>\*</del> | Monotheism: Natural Instinct<ins>±</ins> |
| O112 (held) | 31:34 subtitle | Things We May and May Not Know<del>\*</del> | Things We May and May Not Know<ins>±</ins> |
| O113 (held) | 33:7 subtitle | Muhammad Pledges to Support God's Messenger of the Covenant<del>\*</del> | Muhammad Pledges to Support God's Messenger of the Covenant<ins>±</ins> |
| O114 (held) | 33:21 subtitle | The Prophet's Courage<del>\*</del> | The Prophet's Courage<ins>±</ins> |
| O115 (held) | 33:40 subtitle | Not The Final Messenger<del>\*</del> | Not The Final Messenger<ins>±</ins> |
| O116 (held) | 33:56 subtitle | During the Prophet's Life<del>\*</del> | During the Prophet's Life<ins>±</ins> |
| O117 (held) | 33:63 subtitle | End of the World Revealed<del>\*</del> | End of the World Revealed<ins>±</ins> |
| O118 (held) | 34:21 subtitle | The Objective: Do We Believe in the Hereafter?<del>\*</del> | The Objective: Do We Believe in the Hereafter?<ins>±</ins> |
| O119 (held) | 34:43 subtitle | Mathematical Miracle of the Quran<del>\*</del> | Mathematical Miracle of the Quran<ins>±</ins> |
| O120 (held) | 34:46 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O121 (held) | 35:12 subtitle | Appreciating God's Greatness<del>\*</del> | Appreciating God's Greatness<ins>±</ins> |
| O122 (held) | 35:14 subtitle | The Idols Utterly Powerless<del>\*</del> | The Idols Utterly Powerless<ins>±</ins> |
| O123 (held) | 35:24 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O124 (held) | 36:13 subtitle | Rejecting The Messengers: A Tragic Human Trait<del>\*</del> | Rejecting The Messengers: A Tragic Human Trait<ins>±</ins> |
| O125 (held) | 36:26 subtitle | The Righteous Go Straight to Paradise<del>\*</del> | The Righteous Go Straight to Paradise<ins>±</ins> |
| O126 (held) | 36:30 subtitle | Ridiculing The Messengers: A Tragic Human Trait<del>\*</del> | Ridiculing The Messengers: A Tragic Human Trait<ins>±</ins> |
| O127 (held) | 37:102 subtitle | Satanic Dream<del>\*</del> | Satanic Dream<ins>±</ins> |
| O128 (held) | 38:41 subtitle | The Devil Afflicts Job<del>\*</del> | The Devil Afflicts Job<ins>±</ins> |
| O129 (held) | 39:5 subtitle | The Shape of the Earth<del>\*</del> | The Shape of the Earth<ins>±</ins> |
| O130 (held) | 39:45 subtitle | The Greatest Criterion<del>\*</del> | The Greatest Criterion<ins>±</ins> |
| O131 (held) | 39:67 subtitle | Greatness of God<del>\*</del> | Greatness of God<ins>±</ins> |
| O132 (held) | 40:11 subtitle | God ALONE: The Disbelievers Suffer Two Deaths<del>\*</del> | God ALONE: The Disbelievers Suffer Two Deaths<ins>±</ins> |
| O133 (held) | 40:34 subtitle | Who is the Last Messenger?<del>\*</del> A Tragic Human Trait | Who is the Last Messenger?<ins>±</ins> A Tragic Human Trait |
| O134 (held) | 40:60 subtitle | Supplication: A Form of Worship<del>\*</del> | Supplication: A Form of Worship<ins>±</ins> |
| O135 (held) | 40:78 subtitle | God's Authorization for the Quran's Mathematical Miracle<del>\*</del> | God's Authorization for the Quran's Mathematical Miracle<ins>±</ins> |
| O136 (held) | 41:41 subtitle | Mathematical Miracle of the Quran<del>\*</del> | Mathematical Miracle of the Quran<ins>±</ins> |
| O137 (held) | 41:43 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O138 (held) | 41:53 subtitle | A Great Prophecy<del>\*</del> | A Great Prophecy<ins>±</ins> |
| O139 (held) | 42:21 subtitle | The Idols: Innovating New Religious Laws<del>\*</del> | The Idols: Innovating New Religious Laws<ins>±</ins> |
| O140 (held) | 42:24 subtitle | God Erases the Falsehood and Affirms the Truth<del>\*</del> | God Erases the Falsehood and Affirms the Truth<ins>±</ins> |
| O141 (held) | 43:36 subtitle | Invisible, Devilish, Companions<del>\*</del> | Invisible, Devilish, Companions<ins>±</ins> |
| O142 (held) | 43:61 subtitle | Jesus and the End of the World<del>\*</del> | Jesus and the End of the World<ins>±</ins> |
| O143 (held) | 44:10 subtitle | The Smoke: A Major Prophecy<del>\*</del> | The Smoke: A Major Prophecy<ins>±</ins> |
| O144 (held) | 44:13 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O145 (held) | 44:56 subtitle | The Righteous Do Not Really Die<del>\*</del> | The Righteous Do Not Really Die<ins>±</ins> |
| O146 (held) | 45:6 subtitle | Which Hadith?<del>\*</del> | Which Hadith?<ins>±</ins> |
| O147 (held) | 46:6 subtitle | The Idols Disown Their Worshipers<del>\*</del> | The Idols Disown Their Worshipers<ins>±</ins> |
| O148 (held) | 46:10 subtitle | Rabbi Judah the Pious<del>\*</del> | Rabbi Judah the Pious<ins>±</ins> |
| O149 (held) | 46:15 subtitle | 40: The Age of Decision<del>\*</del> | 40: The Age of Decision<ins>±</ins> |
| O150 (held) | 46:29 subtitle | Believers Among the Jinns<del>\*</del> | Believers Among the Jinns<ins>±</ins> |
| O151 (held) | 46:35 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O152 (held) | 47:18 subtitle | End of the World<del>\*</del> | End of the World<ins>±</ins> |
| O153 (held) | 47:38 subtitle | Warning to the Arabs<del>\*</del> | Warning to the Arabs<ins>±</ins> |
| O154 (held) | 48:28 subtitle | The Great Prophecy<del>\*</del> | The Great Prophecy<ins>±</ins> |
| O155 (held) | 57:22 subtitle | Profound Fact<del>\*</del> | Profound Fact<ins>±</ins> |
| O156 (held) | 63:6 subtitle | The Myth of Intercession Shattered<del>\*</del> | The Myth of Intercession Shattered<ins>±</ins> |
| O157 (held) | 65:12 subtitle | Seven Universes And Seven Earths<del>\*</del> | Seven Universes And Seven Earths<ins>±</ins> |
| O158 (held) | 67:2 subtitle | The Purpose of Our Life<del>\*</del> | The Purpose of Our Life<ins>±</ins> |
| O159 (held) | 72:19 subtitle | God's Messenger of the Covenant<del>\*</del> | God's Messenger of the Covenant<ins>±</ins> |
| O160 (held) | 110:0 subtitle | (Last Sura Revealed)<del>\*</del> | (Last Sura Revealed)<ins>±</ins> |

