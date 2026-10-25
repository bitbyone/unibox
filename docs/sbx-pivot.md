# Unibox jako tenká nadstavba nad sbx – implementační plán

## Cíl

Konfiguraci sandboxu vlastní sbx: `*.sbxenv.yaml` plus kity v3 v týmovém repu.
Unibox nemá vlastní formát. Bere libovolný sbxenv tak, jak je, a přidává jen to, co sbx neumí:

1. **Osobní hodnoty**: `vars.toml` → `--env-args-file`, aby se `env.args` nemusely vypisovat při každém create.
2. **Loginy**: wizard, který zjistí, které secrety kity potřebují, a nechá je uložit přes `sbx secret set`.
3. **Tasky**: repo do sandboxu naživo přes `sbx mount` (direct) nebo jako klon uvnitř, agent se pustí v něm, stav se vede.
4. **Pojmenované porty**: osobní mapování portů na hostu, publikované a rušené podle jména.
5. **Doctor, drift, recreate**: řekne, co chybí nebo se změnilo a co z toho plyne.
6. **herdr** zůstane, jak je.

Kdo unibox nechce, použije tytéž soubory přes čisté `sbx env create`. Unibox nesmí být podmínkou.

## Principy

- **Deterministicky pro jakýkoli sbxenv.** Unibox sbxenv nemění, negeneruje ani neslučuje s vlastními soubory. Smí ho jen číst (args, kity, název) a předat sbx.
- **Žádná runtime abstrakce.** Ruší se `Runtime` a msb a zůstává konkrétní sbx klient. AGENTS.md: "Nothing above the adapter names sbx" přestává platit.
- **Tvrdé pravidlo o loginech platí dál.** Hodnotu zadává uživatel přímo do `sbx secret set` s TTY zděděnou od uniboxu, takže unibox ji vůbec nevidí.
- **Stav je jen uniboxu.** Tasky, mounty, runy a otisk konfigurace, ze které sandbox vznikl. Co ví sbx (`sbx ls`, `sbx secret ls`), se neduplikuje.

## Co zmizí

| Oblast | Soubory | Proč |
| --- | --- | --- |
| Formát profilu | `profiles.go`, `profileedit.go`, `profilepath.go`, `migrations.go`, `validate.go`, `diff.go`, `changes.go` (část) | nahrazuje ho sbxenv |
| Šablony a scaffold | `template.go`, `scaffold.go`, `templates/{managed,user,profile,build}`, `cli/template.go` | sdílení řeší týmové repo s kity |
| Generování kitů a credentials | `sbx_kit.go`, `sbx_auth.go` (zápis `credentials.yaml`) | kity píše tým, bindingy jsou v sbxenv |
| Build image | `build.go`, `@install` runner | dělá ho sbx z kitů |
| msb | `msb*.go`, cgo tag | konec runtime agnostiky |
| Runtime rozhraní | `runtime.go` (Runtime, Mounter, Authenticator, …) | jeden konkrétní `sbx` klient |
| Příkazy nad profilem | `network`, `service`, `agent`, `secret source`, `profile set/unset`, `co-author`, `instructions` | řeší kity, `sbx policy`, `sbx ports` a `sbx secret` |
| Seedy agentů | `templates/seeds` | konfiguraci agenta dodává kit (viz `copilot-config`) |

Zhruba 60 % kódu. Mažu až ve fázi 5, aby se do té doby dalo porovnávat chování.

## Co zůstane a upraví se

| Oblast | Soubory | Úprava |
| --- | --- | --- |
| Tasky | `tasks.go`, `repos.go`, `api/types.go` | `Sandbox` místo profilu odkazuje na env (soubory, args a otisk). Mount a clone zůstávají. |
| Mounty | `sbx.go:469-485` (`Mount`/`Unmount`) | beze změny, jen přesun do `sbx` klienta |
| Příprava agenta | `agents.go`, část `manager.go` | trust (copilot `trustedFolders`, claude `~/.claude.json`, codex `config.toml`) pro adresář tasku |
| herdr | `herdr.go`, `cli/herdr.go` | beze změny |
| Úklid | `destroy.go`, `prune.go` | jen sandboxy, které založil unibox |
| Kontext, config, lock | `context.go`, `config.go`, `lock_unix.go` | `default_env` místo `default_profile` |
| Vars | `vars.go`, `cli/vars.go` | klíčované podle env (viz níže) a výstup do `--env-args-file` |
| Porty | `ports.go`, příkazy `port` | Jen z `config.toml` uživatele místo profilu, viz "Porty" níže. Stejná syntaxe a příkazy. |
| Doctor | `cli/doctor.go` | nové kontroly (viz fáze 4) |
| Testy | `internal/testenv`, fake runtime | fake `sbx` na PATH místo fake runtime |

## Model

```
~/.config/unibox/config.toml
  default_env = "my2n-dev"
  [envs.my2n-dev]
  files = ["~/workspace/my2n/ai-transformation/agent-sandboxes/sandboxes/my2n-dev.sbxenv.yaml"]
  name  = "my2n-dev"                 # --name pro sbx

~/.config/unibox/vars.toml
  [envs.my2n-dev.args]
  artifactoryUser = "tobola"
  memory = "16g"
```

- **Env** je pojmenovaná sada sbxenv souborů (pořadí = pořadí slučování v sbx) a osobních args.
  Vznikne přes `unibox env add <name> <file...>`. `unibox init -i` tohle provede jako první krok.
- **Args:** před každým `sbx env create` se z `vars.toml` zapíše dočasný soubor pro `--env-args-file` (mód 0600, po použití se smaže).
  Secrety tam nepatří, vars.toml je odmítne (hodnota začínající `secret:` je chyba).
- **Otisk:** hash obsahu všech souborů env, jejich lokálních kitů (adresáře z `kits[].source`) a args.
  Uloží se do stavu sandboxu při create. Liší-li se při `run`, unibox to oznámí a nabídne `unibox sandbox recreate`, nic sám neschvaluje.

## Porty

Porty zůstávají pojmenované, ale patří uživateli, ne týmu. Jsou v
`~/.config/unibox/config.toml` u env a nikdy v sbxenv, protože host port je
věc stroje (co už na něm běží, jaké číslo zná prohlížeč).

```
[envs.my2n-dev.ports]
web      = "3000:5173"                     # publikovaný při startu
storybook = { port = "6006:6006", default = "off" }
```

- Syntaxe a chování jako dnes (`ports.go`): spec je `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`, jen loopback, `default` on/off.
- `unibox port set|unset|ls` mění `config.toml`; `unibox port publish|unpublish <name>` (i `--map` pro jednorázové mapování) přepíná na běžícím sandboxu přes `sbx ports --publish|--unpublish`.
- Při `run` a po recreate unibox srovná publikované porty se seznamem (dnešní `syncPorts`).
- Spravuje jen porty, které sám publikoval (vede je ve stavu). Porty z kitů (`port@1`, třeba node dev servery na náhodném portu) ani ručně publikované přes `sbx ports` nepřepisuje ani neruší; `port ls` je ukáže zvlášť bez jména.
- Kolize host portu (obsazený jiným sandboxem nebo procesem) je chyba se jménem portu a radou, jak ho přemapovat.

## Fáze

### Fáze 0: Ověření předpokladů v sbx (před kódem)

Na throwaway sandboxu (`--name utest`), po testu `sbx rm`:

1. `sbx env plan` vrací strojově čitelný výstup (`--json`?). Pokud ne, unibox si `env.args` a `kits` čte z YAML sám, jen read-only.
2. **Agent přes `sbx exec` = agent přes `sbx run`.** Ověřit, že `sbx exec -it -w DIR sandbox copilot …` dostane totéž prostředí (proxy env z PID 1, `BASH_ENV`, PATH z mise), že se injektují tokeny a že agentovi funguje auth.
3. Odkud vzít příkaz agenta: `sbx kit inspect` workloadu, nebo jen známé agenty (copilot, claude, codex) jako dnes v `agents.go`.
4. Jak zjistit, které secrety sandbox potřebuje: `credential@1` v kitech plus `bindings` v sbxenv, a jak je porovnat se `sbx secret ls`, aniž by se četla hodnota.
5. `sbx mount` na cestu, kde je stejná host cesta (`$PWD:$PWD`): rodičovské adresáře vytvořené jako root a jejich dopad na agenta.
6. Dva tasky naráz v jednom sandboxu (dva `exec -it`) a sdílená Gradle cache (zámky).

Výstupem fáze 0 je doplněná sekce v `docs/design.md`. Do té doby se nic nemaže.

**Hotovo (2026-10-25, sbx 0.49.2)**, podrobnosti v `docs/design.md`, sekce "sbx as the configuration":

1. `plan` je jen text. Unibox čte `args`, `kits` a `bindings` z YAML sám, `plan` pouští jen kvůli exit kódu (chybějící nebo neznámý arg = 1).
2. `exec` dává agentovi stejné prostředí jako `run`, auth funguje. Příkazy se spouštějí přes bash, ne `sh` (dash nečte `BASH_ENV`).
3. Příkaz agenta: `sbx kit inspect --json` workload kitu, `sandbox.entrypoint` + `sandbox.command.default`.
4. Potřebné secrety: `credentials[].service` z `kit inspect --json` všech kitů (`optional` jen z YAML kitu), uložené: `sbx secret ls --json`.
5. Rodiče mountu vlastní root (755), agent vedle repa nezapíše. Git i commit fungují, `.git/hooks` je zapisovatelný.
6. Dva agenti v jednom sandboxu naráz fungují. Zámky Gradle cache se netýkají uniboxu.

Navíc: mounty přežijí stop/start a chybějící host cesta start shodí, proto `task rm` nejdřív odmountuje. Výpis mountů sbx nemá, jen `/proc/self/mountinfo` v běžícím sandboxu. První `create` jde se zděděnou TTY, aby uživatel bindingy schválil sám, unibox za něj `--force` nedává.

### Fáze 1: Env místo profilu

- Typ `Env` (files, name, args) a jeho načtení z `config.toml`, `unibox env add|ls|rm|show`.
- Čtení sbxenv: jen `env.args` (deklarace, required, default), `kits`, `bindings`. Neznámá pole se ignorují, aby nové verze sbx unibox nerozbily.
- `vars.go` překlíčovat na `[envs.<name>.args]` a vytvořit generátor `--env-args-file`.
- `unibox sandbox create [env]` → `sbx env create --name … --env-args-file … <files…>`. Chybí-li povinný arg, zastaví se se jménem argu a příkazem `unibox var set`.
- Otisk a jeho zápis do stavu.
- Porty: `ports.go` přepojit z profilu na `[envs.<name>.ports]`, stav publikovaných portů u sandboxu.

### Fáze 2: Tasky nad sbx

- `unibox run [-e env] [--mount|--clone] [dir…]`:
  1. zajistit sandbox (create při prvním použití),
  2. `--mount` (výchozí pro existující lokální adresář): `sbx mount <sb> <host>:<host>` read/write a zápis do stavu tasku,
  3. `--clone`: dnešní cesta přes `ResolveRepositories` (klon uvnitř sandboxu, push ven),
  4. připravit trust agenta pro adresář tasku,
  5. spustit agenta přes `sbx exec -it -w <dir> <sb> <agent> <args>` a zaznamenat run.
- `unibox resume <task>`, `task ls`, `task rm` (→ `sbx umount` a smazání ze stavu), `add-repo` jako dnes.
- `sandbox rm` → umount všech tasků, `sbx rm`, smazání ze stavu. Sandbox, který unibox nezaložil, se nemaže bez `--force`.
- **Bezpečnost direct mountu:** při prvním `--mount` jednou za repo vypsat, že agent může zapisovat do `.git/hooks` a `.git/config`, které se pak spustí na hostu.
  Volitelně `--protect-git`: `.git` navrch přimountovat jen pro čtení (agent pak necommituje, viz fáze 0, bod 5).

**Hotovo** (ověřeno proti sbx 0.49.2 na throwaway sandboxu). Odchylky od plánu:

- Mazání profilů, msb, šablon a buildů proběhlo už tady, ne až ve fázi 5; jádro bez nich nešlo přepojit.
- Konfigurace zůstává v `config.yaml` (envy pod `envs:`, porty u envu), args ve `vars.toml` pod `[envs.<name>]`. Klíče z v1 (`runtimes.sbx.ssh_agent`) se přečtou, `environments` se zahodí.
- Mount jde na `/work/<task>/<repo>`, ne na stejnou cestu jako na hostu: tasky tak mají jeden tvar pro mount i klon.
- `sandbox create --adopt` převezme sandbox se stejným jménem, který unibox nezaložil; takový se nikdy nerecreatuje a `destroy` ho jen zapomene.
- Mezi jádrem a sbx je rozhraní `Runtime`, aby testy jely proti falešnému runtime v Go místo skriptů na PATH.
- `--protect-git` není; místo něj se při prvním mountu adresáře jednou řekne, že agent může psát do `.git`, a doporučí `--clone`.
- Preflight se na chybějící args neptá, jen řekne `unibox var set`; ptaní přijde s wizardem ve fázi 3. `unibox init` do té doby jen odkáže na `unibox env add`.
- Nedokončené vytvoření (sbx odmítne bez schválení) se při dalším `create`/`run` zkusí znovu, a pokud sbx sandbox už udělal, jen se dopřipraví.

### Fáze 3: Loginy a wizard

- `unibox login [env]`: ze sbxenv a kitů zjistí potřebné secrety, porovná se `sbx secret ls` a pro chybějící spustí `sbx secret set <name>` se zděděnou TTY.
  Pro známé případy (keychain apod.) jen vypíše návod jako README týmového repa; hodnotu nikdy nepřenáší sám.
- `unibox init -i` jako walk: sbx (verze, ssh agent, `env.rememberHostCommands`), Env (soubory), Values (chybějící args → vars.toml), Logins, Create, Run.
  Kroky volají tytéž příkazy jako dnes (`wizard.go` zůstává jako kostra).

**Hotovo.** `unibox login ls|set` (podle konvence podstatné jméno a sloveso), `set` umí `--command` pro keychain a hodnotu z pipe; secrety jsou vždy globální, protože `sbx env rm` maže ty sandboxové. `init` je nový pětikrokový walk (sbx, Env, Values, Logins, Sandbox) místo `wizard.go`. Chybějící args se ptá `run` i `sandbox create`. Kit s povinným vlastním argem (`2n-artifactory`) se čte s `--kit-arg` z hodnot env, jinak by ho `sbx kit inspect` nepřečetl. `env.rememberHostCommands` walk neřeší, nastavuje se v sbx.

### Fáze 4: Doctor a drift

- doctor kontroluje:
  - verzi sbx proti nejnižší podporované,
  - že soubory env existují a `sbx env plan` projde,
  - povinné args ve vars.toml,
  - přítomnost secretů (bez hodnot),
  - `rememberHostCommands`, rozhodnutí o ssh agentovi,
  - osiřelé mounty (ve stavu, ale ne v sandboxu, a naopak),
  - sandboxy ve stavu, které už sbx nezná.
- Při `run` se otisk porovná s uloženým, změna se oznámí s doporučením recreate a tasky se po recreate znovu namountují.

**Hotovo.** Nejnižší verze sbx je 0.49.2 (`MinSBXVersion`, ta ověřená); starší doctor hlásí jako chybu. `sbx env plan` se pouští až s kompletními args, ukáže jeho souhrnný řádek nebo první řádek chyby. `env.rememberHostCommands` zapnuté je varování, ne chyba. Osiřelé mounty se čtou z `/proc/self/mountinfo` jen v běžícím sandboxu; kořen virtiofs mountu je cesta na hostu, takže rada je přesný `sbx umount <sandbox> host:cíl`. Chybějící mount tasku je varování: příští `resume` ho namountuje znovu, stejně jako po recreate (test).

### Fáze 5: Úklid kódu a dokumentace

- Smazat vše z tabulky "Co zmizí", `api` zvednout na v2 (`Sandbox.Profile` → `Sandbox.Env`).
- Přepsat README, `docs/design.md` (ponechat ověřená fakta o sbx), web a AGENTS.md (mapa balíčků, rozhodnutí).
- Release `v2.0`. Starý stav (`state.json` s profily) se nečte; při startu se jednou ohlásí, že sandboxy z v1 je potřeba odstranit přes `sbx rm`.

## Testy

- Fake `sbx` na PATH (`testenv.WriteExecutable`) zapisuje volání do souboru a vrací připravené JSON odpovědi. Testy, které ho používají, jsou sériové.
- Unit testy čtení sbxenv: args, required/default, slučování víc souborů, neznámá pole.
- Otisk: změna souboru, kitu nebo argu ho změní; změna mimo env ne.
- Tasky: mount a umount ve správném pořadí, task se nesmaže, když umount selže (jako dnes).
- Ručně proti skutečnému sbx: throwaway sandbox z `agent-sandboxes`, `run --mount`, `resume`, `task rm`, `sandbox rm`.

## Rizika a otevřené otázky

- **Agent přes `exec`:** ve fázi 0 ověřeno, že se chová jako přes `run`. Fallback (`sbx run` a `cd` přes `BASH_ENV`) zůstává, kdyby to nová verze sbx změnila.
- **sbx se rychle mění:** `sbxList` tolerantní vůči tvaru JSON zůstává a minimální verze sbx se hlídá v doctor.
- **Více sandboxů nad stejnou read/write Gradle cache:** zámky. Netýká se uniboxu, ale týmového repa.
- **Kolize jmen:** sandbox založený ručně přes `sbx env create` se stejným jménem. Unibox ho převezme jen s `--adopt`.
