# Backend message translation source

The portal repository owns these backend-namespaced client catalogs. `en.json`
is extracted from CitadelOpsBackend PR95 at e8e34993 (282 explicit keys), not an
independently edited translation. The server sends keys, scalar parameters and
safe context descriptors; each viewer selects a pack locally.

All 25 non-English backend packs were authored by the implementation model in
this task on 2026-09-20. No source was uploaded to a translation service. They have
key/ICU-argument validation; independent native-speaker review remains pending.
Coverage records must distinguish authored translations, fallback and review.

## Glossary and register

- CitadelOps, Ops Credits, Google, US1, RFC3339, JSON and API paths retain their
  identity. Generic credits may use the language's normal financial noun.
- API identifiers (commanderID, savedAtUnix, worldId, CRA, AIN, admin/edit,
  players/alliances) remain literal code, even inside translated prose.
- German uses informal singular `du` and imperative `Gib`/`Prüfe`. French uses
  polite `vous` and imperatives `Saisissez`/`Vérifiez`. Spanish uses informal
  singular `tú` and imperatives `Introduce`/`Revisa`. Italian uses informal
  singular `tu` with `Inserisci`/`Controlla`. Portuguese follows the official game’s Brazilian vocabulary/register
  with `você`, `Digite`, `Salvar`, `senha` and `usuário`.
- Runtime means an executing account instance; launch means a captured Rift
  launch record. Tombstone means a deletion marker, not an in-game grave.
- A hosted tenant is an access-isolated account; it is not a player alliance.
- Credential validation must tell the user what to fix. Conflict messages retain
  expected/current revisions and record context. Never translate a diagnostic
  string by reverse matching it.

Every translated template must retain exactly its source ICU arguments. Context
labels translate independently; their identifiers remain scalar user data.
Source hash provenance and reproducible sync are maintained by the catalog scripts.

Official game glossary checked by Adrian against v4357 `battlelog`: de
Kampfbericht, fr Rapport de bataille, es Informe de combate, it Rapporto di
battaglia, pt Relatório de combate, nl Gevechtsverslag. These terms apply to actual
game battle reports, not generic training payloads. Source evidence is the
versioned official corpus used by the shared game-localization catalog.
Dutch and Swedish use informal singular address with direct imperatives.
Danish uses informal direct address. Official v4357 `battlelog` is Kamprapport;
launch records use “start”, and tombstones use “slettemarkør” (deletion marker).
Norwegian uses Bokmål with informal direct address; official v4357 `battlelog`
is Kamprapport. Deletion markers are “slettemarkører”.
Polish uses direct singular address. Official v4357 `battlelog` is Raport z bitwy.
Variable maxima use neutral limit phrasing to avoid incorrect noun inflection;
technical field names, strict positivity, non-negativity and byte limits remain
unchanged in meaning.
Turkish uses polite plural imperatives. Official v4357 `battlelog` is Savaş raporu.
“Başlatma” denotes a captured launch record; “silme işareti” is a deletion marker.
Russian uses polite plural imperatives. Official v4357 `battlelog` is Отчет о
сражении, inflected in sentences. Attack validity is a captured-record validity
value, not an expiry duration; sharing a spy report grants access and does not
claim public publication.

Portuguese terminology also checked against official v4357
`generic_login_password`, `generic_login_loginname`,
`dialog_battleLogDetail_wave` and `dialog_troopPreset_savePreset_selectSingleWave_tt`.
Attack waves are ondas, not vagas. The public locale remains pt, matching the
official service; no invented regional locale or game-language change is made.
Chinese variants are separately authored with Simplified/Traditional terminology.
Official v4357 battlelog is 战报/戰報; dialog_openSpyReport_Tooltip confirms
谍报/間諜報告. Attack support-tool and slot terms follow
`dialog_attack_rework2022_slot_supportTools_tooltip` and its sibling slot keys
(支援型武器栏位 / 支援武器欄位). Technical IDs and byte limits stay literal.
Arabic uses Modern Standard Arabic UI imperatives. Variable counts use neutral
count/limit phrasing where a fixed noun form would be incorrect. Explicitly
positive values remain distinct from non-negative values. Catalogs contain no
hand-authored bidi controls: the shared formatter isolates rendered arguments
and complete context segments, leaving stored identifiers unchanged.

Finnish uses direct imperatives and neutral technical validation wording. Official
v4357 battlelog is Taisteluraportti. A runtime is a suoritusinstanssi and a
tombstone is a poistomerkintä; neither denotes an in-game building.

Japanese uses polite UI instructions and concise validation statements. Official
v4357 terminology: 戦闘結果, スパイ活動レポート, サポート兵器 and 波状攻撃.
Launch records are 起動記録; deletion markers are 削除マーカー.

Korean uses polite UI instructions and declarative validation messages. Official
v4357 terminology: 전투 결과, 첩보 보고서, 지원 병기, and 공격 횟수. Unknown
identifier endings use paired particles or label phrasing rather than modifying IDs.

Czech uses informal singular instructions (Zadej/Zkontroluj). Official game
terms include Záznam z bitvy, záznam ze špionáže and podpůrný nástroj. Variable
counts use count/limit labels when a fixed noun form would depend on the value.

Slovak uses informal singular instructions (Zadaj/Skontroluj), with official
Správa o boji, správa o špionáži and nástroje na podporu. Launch records and
deletion markers remain technical records, not game entities.

Romanian uses informal singular instructions (Introdu/Verifică). Official
Raport de luptă, raport de spionaj and unealtă de asistență guide game terms.
A tenant is an isolated account, and a tombstone is a marcaj de ștergere.

Greek uses informal singular instructions. Official Αναφορά μάχης, αναφορά
κατασκοπείας, εργαλεία υποστήριξης and κύμα επιθέσεων guide game terms.
Non-negative values (μη αρνητικές) stay distinct from positive values (θετικό).

Hungarian uses informal singular instructions (Add meg/Ellenőrizd). Game prose
uses the official csatáról szóló jelentés, kémjelentés and támogatási eszköz
concepts; identifier parameters are not inflected or rewritten.

Bulgarian uses informal singular instructions (Въведи/Провери). Official
Отчет от битка, отчет от шпионаж and помощен инструмент guide game terms.
Count labels avoid fixed plural forms, and positive/non-negative remain distinct.

Lithuanian uses informal singular instructions (Įvesk/Patikrink). Official
Mūšio ataskaita, šnipinėjimo ataskaita and paramos įrankiai guide game terms.
Variable counts use count/limit phrasing; technical identifiers remain literal.
