# Relatório de Análise Técnica — Updash

**Data da Análise:** Agosto de 2026  
**Repositório:** `github.com/lgldsilva/updash`  
**Escopo:** Análise de integridade, consistência arquitetural, bugs, cenários de falha ocultos e compatibilidade multiplataforma (Linux, macOS, Windows).

---

## 1. Sumário Executivo

Durante a análise estática e dinâmica do codebase do **`updash`**, foram identificados 14 pontos de melhoria distribuídos em quatro categorias principais:
1. **Bugs Críticos e Funcionais:** Duplicações de execução, filtros quebrados em CLI, bloqueio de caracteres em inputs interativos e falha de self-update no Windows.
2. **Inconsistências Arquiteturais:** Código inalcançável (dead code), scanners que não executam checagens reais (mock estático em produção), sobrescrita indevida de campos de modelo (`CurrentVer`) e comportamento discrepante de atualizações em lote no Windows.
3. **Casos Limítrofes e Compatibilidade:** Problemas de permissão no Docker (root vs non-root), dependência de comandos Unix (`ls`) em ambientes Windows, renderização vertical descontrolada no TUI e políticas de retenção agressivas por `mtime` de diretório.
4. **Metadados e Scripts:** Avisos obsoletos de migração e inconsistências de cobertura em scripts de desenvolvimento.

---

## 2. Detalhamento dos Problemas Identificados

### Categoria A: Bugs Críticos & Funcionais

#### A.1 Duplicação do Scanner do Pacman
- **Arquivo:** [`internal/scanner/scanner.go`](file:///storage/Projetos/updash/internal/scanner/scanner.go#L77-L78)
- **Linhas afetadas:** 77-78
- **Descrição:** A checagem `{plat.HasPacman || plat.HasYay, &PacmanSource{}}` foi adicionada duas vezes consecutivas em `appendPlatformSources`.
- **Impacto:** Em sistemas Arch Linux / Manjaro, o scan do Pacman é disparado duas vezes em paralelo, duplicando processamento e gerando sumarizações concorrentes/duplicadas.
- **Correção Proposta:** Remover a linha duplicada (linha 78).

#### A.2 `--only` do CLI não casa com categorias de itens filhas (Ex: `gh-ext`)
- **Arquivo:** [`internal/cli/cli.go`](file:///storage/Projetos/updash/internal/cli/cli.go#L432-L447)
- **Linhas afetadas:** 432-447 (`itemMatchesFilter`)
- **Descrição:** `itemMatchesFilter` compara `s.Category` (a categoria do sumário pai `CatAI`), `s.Label` e `it.Name`, mas **não compara `it.Category`**.
- **Impacto:** O item **Gh Extensions** possui categoria `CatGHExt`, mas pertence ao scanner `AIInfraSource` (`CatAI`). Ao rodar `updash --update --only gh-ext`, o comando retorna zero itens.
- **Correção Proposta:** Adicionar `strings.EqualFold(string(it.Category), o)` na validação de `itemMatchesFilter`.

#### A.3 `CatApk` ausente em `CategoryNeedsElevation`
- **Arquivo:** [`internal/elevate/needs.go`](file:///storage/Projetos/updash/internal/elevate/needs.go#L16-L24)
- **Linhas afetadas:** 17-18
- **Descrição:** `CategoryNeedsElevation` não lista `model.CatApk`, apesar de `batchApkUpgrade` invocar `elevate.Sudo` quando `os.Geteuid() != 0`.
- **Impacto:** Em containers Alpine ou ambientes não-root com `apk`, o CLI e TUI não preparam a sessão de elevação antes de executar o lote.
- **Correção Proposta:** Adicionar `model.CatApk` ao `switch cat` em `CategoryNeedsElevation`.

#### A.4 Bloqueio indevido de caracteres `?` e `/` em inputs do TUI
- **Arquivo:** [`internal/tui/update.go`](file:///storage/Projetos/updash/internal/tui/update.go#L138) e [`internal/tui/update.go`](file:///storage/Projetos/updash/internal/tui/update.go#L172)
- **Linhas afetadas:** 138 (`handlePasswordKey`) e 172 (`handleFilterKey`)
- **Descrição:**
  - `handlePasswordKey`: `if len(key) == 1 && key != "?"` impede explicitamente a digitação de `?` na senha do sudo.
  - `handleFilterKey`: `if len(key) == 1 && key != "/" && key != "?"` impede a digitação de `/` e `?` no filtro.
- **Impacto:** Usuários com senhas contendo `?` não conseguem autenticar. Usuários não conseguem filtrar pacotes escopados (ex: `@anthropic-ai/claude-code`).
- **Correção Proposta:** No modal de senha, remover a trava `key != "?"`. No modal de filtro, remover as travas `key != "/"` e `key != "?"`.

#### A.5 Auto-upgrade no Windows falha por bloqueio de arquivo em execução
- **Arquivo:** [`internal/upgrade/upgrade.go`](file:///storage/Projetos/updash/internal/upgrade/upgrade.go#L406) e [`internal/upgrade/startup.go`](file:///storage/Projetos/updash/internal/upgrade/startup.go#L44-L48)
- **Linhas afetadas:** `replaceRunningBinary` (upgrade.go:406) e `selfUpdateAllowed` (startup.go:44-48)
- **Descrição:**
  1. `replaceRunningBinary` faz `os.Rename(tmp, self)` diretamente. No Windows, o executável em execução é travado pelo SO (`ERROR_ACCESS_DENIED`).
  2. `selfUpdateAllowed` valida apenas se o caminho está em `$HOME/.local/bin`. No Windows, instalações ficam em caminhos como `%LOCALAPPDATA%`, `%USERPROFILE%\scoop\shims`, `%ProgramData%\chocolatey` ou `%USERPROFILE%\bin`.
- **Impacto:** Auto-upgrade quebra invariavelmente no Windows ou é bloqueado sob a justificativa de ser "package-managed".
- **Correção Proposta:** No Windows, usar a estratégia de renomear `self -> self.old` antes de mover `tmp -> self`, agendando a exclusão de `self.old` na inicialização subsequente; e flexibilizar `selfUpdateAllowed` no Windows para diretórios do usuário.

---

### Categoria B: Inconsistências Arquiteturais e Lógicas

#### B.1 `runSDKMANUpgrade` no `updater.go` é código morto (unreachable)
- **Arquivo:** [`internal/updater/updater.go`](file:///storage/Projetos/updash/internal/updater/updater.go#L761-L767) vs [`internal/scanner/scanner.go`](file:///storage/Projetos/updash/internal/scanner/scanner.go#L125)
- **Linhas afetadas:** 761-767 (`runSDKMANUpgrade`)
- **Descrição:** O `CatSDKMAN` foi categorizado estritamente como `IsCleanupCategory`. O scanner `SDKMANSource` só existe em `appendCleanupSources` e gera apenas itens de limpeza (`StatusCleanCandidate`). O fluxo de update nunca recebe itens do SDKMAN.
- **Impacto:** O método `runSDKMANUpgrade` (que executa `sdk upgrade`) nunca é chamado.
- **Correção Proposta:** Decidir entre (a) adicionar suporte real a update do SDKMAN (registrando um scanner de atualização em `appendLanguageSources`) ou (b) remover o código morto em `updater.go`.

#### B.2 `CargoSource` não detecta crates desatualizadas
- **Arquivo:** [`internal/scanner/rustup.go`](file:///storage/Projetos/updash/internal/scanner/rustup.go#L67-L76)
- **Linhas afetadas:** 67-76 (`CargoSource.Scan`)
- **Descrição:** O scanner apenas verifica se `cargo-install-update` está presente e retorna `StatusOK` fixo ("cargo-install-update available").
- **Impacto:** Nenhuma ferramenta instalada via `cargo install` é checada contra versões mais recentes.
- **Correção Proposta:** Executar `cargo install-update -l` para fazer o parsing real das crates desatualizadas e gerar itens `StatusOutdated`.

#### B.3 Sobrescrita do campo `item.CurrentVer` com diagnóstico de erro
- **Arquivo:** [`internal/updater/updater.go`](file:///storage/Projetos/updash/internal/updater/updater.go#L184) e [`internal/updater/updater.go`](file:///storage/Projetos/updash/internal/updater/updater.go#L257)
- **Linhas afetadas:** 184 (`upgradeOneBrew`) e 257 (`upgradeMASApp`)
- **Descrição:** Quando uma atualização de Homebrew ou MAS falha, `item.CurrentVer` é sobrescrito com `truncatePlainDiagnosis(result.Error)`.
- **Impacto:** A versão atual instalada é perdida e substituída por texto de erro, corrompendo relatórios e saída `--json`.
- **Correção Proposta:** Armazenar o diagnóstico em `item.Log` ou em campo específico, preservando o valor original de `item.CurrentVer`.

#### B.4 Batch updaters no Windows ignoram seleção individual
- **Arquivo:** [`internal/updater/updater.go`](file:///storage/Projetos/updash/internal/updater/updater.go#L491-L514)
- **Linhas afetadas:** 491-514 (`batchWingetUpgrade`, `batchChocoUpgrade`, `batchScoopUpgrade`)
- **Descrição:** Enquanto Homebrew e npm atualizam estritamente os pacotes selecionados, os updaters de Windows executam flags globais (`winget upgrade --all`, `choco upgrade all -y`, `scoop update *`).
- **Impacto:** Se o usuário desmarcar itens no TUI, pacotes desmarcados são atualizados mesmo assim.
- **Correção Proposta:** Quando `items` for um subconjunto, passar os nomes/IDs específicos para `winget upgrade --exact --id <id>`, `choco upgrade <name> -y` e `scoop update <name>`.

#### B.5 `updateSelf()` em `main.go` tem caminho hardcoded e comandos Unix
- **Arquivo:** [`cmd/updash/main.go`](file:///storage/Projetos/updash/cmd/updash/main.go#L458-L493)
- **Linhas afetadas:** 458-493
- **Descrição:** Assume que o repositório fonte está sempre em `~/.config/updash`, ignora erros de `os.UserHomeDir()`, e usa o comando externo `cp` (incompatível com Windows).
- **Correção Proposta:** Descobrir o diretório do repositório a partir do executável atual ou variável de ambiente, tratar erros de home e usar cópia de arquivos nativa em Go (`io.Copy`).

---

### Categoria C: Casos Limítrofes e Compatibilidade Multiplataforma

#### C.1 Falha de permissão ao truncar logs de contêineres Docker
- **Arquivo:** [`internal/cleaner/cleaner.go`](file:///storage/Projetos/updash/internal/cleaner/cleaner.go#L274)
- **Linhas afetadas:** 274 (`cleanContainerLogs`)
- **Descrição:** No Linux, arquivos `/var/lib/docker/containers/*/*-json.log` pertencem a `root:root` (`0600`). Um usuário no grupo `docker` consegue rodar `docker ps` e `docker inspect`, mas `retention.TruncateFileIfOver` falha com `permission denied`.
- **Impacto:** A limpeza de logs de contêiner falha silenciosamente, liberando 0 bytes.
- **Correção Proposta:** Quando a truncagem direta falhar por permissão, tentar truncar via sudo ou comando privilegiado se disponível.

#### C.2 Chamada de comando `ls` no Windows em `goup.go`
- **Arquivo:** [`internal/scanner/goup.go`](file:///storage/Projetos/updash/internal/scanner/goup.go#L33)
- **Linhas afetadas:** 33
- **Descrição:** `GoSource.Scan` executa `execCommand(ctx, "ls", gopath+"/bin")`. No Windows nativo, `ls` não existe por padrão.
- **Correção Proposta:** Substituir a execução de `ls` por `os.ReadDir(filepath.Join(gopath, "bin"))`.

#### C.3 Ausência de Viewport / Scroll vertical no TUI para listas grandes
- **Arquivo:** [`internal/tui/view.go`](file:///storage/Projetos/updash/internal/tui/view.go#L172-L199)
- **Linhas afetadas:** 172-199 (`renderItemTab`)
- **Descrição:** A aba de Logs é limitada por `maxListLines()`, mas as abas de Updates e Cleanup renderizam todas as linhas diretamente no `frame()`.
- **Impacto:** Se houver 30+ pacotes em um terminal de 24 linhas, o layout transborda e corrompe a visualização no AltScreen.
- **Correção Proposta:** Implementar cálculo de viewport com scroll vertical baseado na posição do cursor (`s.Cursor`) e altura útil (`s.maxListLines()`).

#### C.4 Risco de remoção indevida de cache por `mtime` de diretório raiz
- **Arquivo:** [`internal/retention/policy.go`](file:///storage/Projetos/updash/internal/retention/policy.go#L50-L74)
- **Linhas afetadas:** 50-74 (`CollectOldPaths`)
- **Descrição:** Com `maxDepth=1`, o código avalia `fi.ModTime()` de pastas de primeiro nível (ex: `~/.m2/repository/org`). Se a pasta `org` não teve inserção direta de novos arquivos nos últimos 90 dias, a pasta inteira é apagada com `os.RemoveAll`, deletando bibliotecas recentes dentro de subpastas como `org/acme/lib`.
- **Correção Proposta:** Avaliar mtime dos arquivos finais ou descer a árvore recursivamente para verificar se há arquivos recentes antes de deletar a pasta pai.

---

### Categoria D: Metadados e Documentação

#### D.1 Aviso obsoleto de migração no `install.sh`
- **Arquivo:** [`install.sh`](file:///storage/Projetos/updash/install.sh#L226-L228)
- **Linhas afetadas:** 226-228
- **Descrição:** Exibe mensagem sobre "Gitea host" e `UPDASH_SKIP_AUTO_UPGRADE=1`, embora o projeto já esteja no GitHub.
- **Correção Proposta:** Remover as linhas de aviso desatualizadas.

#### D.2 Inconsistência de cobertura no `git-push.sh`
- **Arquivo:** [`git-push.sh`](file:///storage/Projetos/updash/git-push.sh#L5)
- **Linhas afetadas:** 5
- **Descrição:** `COVERAGE_EXCLUDE` inclui `cli`, enquanto o `Makefile` e os testes de CI exigem ≥90% de cobertura em `internal/cli`.
- **Correção Proposta:** Ajustar `COVERAGE_EXCLUDE` para alinhar com `Makefile` e `.ai-standards.env`.

---

## 3. Checklist de Implementação Recomendado

- [ ] **1. Scanner:** Corrigir duplicação do `PacmanSource` em `internal/scanner/scanner.go`.
- [ ] **2. CLI:** Ajustar `itemMatchesFilter` em `internal/cli/cli.go` para validar `it.Category`.
- [ ] **3. Elevação:** Adicionar `model.CatApk` em `internal/elevate/needs.go`.
- [ ] **4. TUI Inputs:** Permitir `?` no modal de senha e `/` e `?` no modal de filtro em `internal/tui/update.go`.
- [ ] **5. Go Scanner:** Substituir `execCommand("ls", ...)` por `os.ReadDir` em `internal/scanner/goup.go`.
- [ ] **6. Modelo / Versões:** Preservar `item.CurrentVer` em falhas de update no Homebrew e MAS (`internal/updater/updater.go`).
- [ ] **7. Windows Updaters:** Respeitar itens específicos em `batchWingetUpgrade`, `batchChocoUpgrade` e `batchScoopUpgrade`.
- [ ] **8. Windows Self-Update:** Implementar rename com fallback `.old` e atualizar `selfUpdateAllowed`.
- [ ] **9. Cargo Scanner:** Implementar parsing real com `cargo install-update -l`.
- [ ] **10. TUI Viewport:** Implementar paginação/janela vertical nas abas de Updates e Cleanup.
- [ ] **11. Scripts:** Limpar avisos obsoletos em `install.sh` e alinhar `git-push.sh`.
- [ ] **12. Validação:** Executar `./scripts/validate.sh` e assegurar que todos os 8 gates passem (build, cross-build, format, vet, tests, coverage >=90%, lint, gosec, vulncheck).
