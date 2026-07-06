<p align="center">
  <a href="#한국어">한국어</a> | <a href="#english">English</a>
</p>

<p align="center">
  <img src="assets/codex_oracle-icon.png" width="96" alt="codex_oracle icon">
</p>

![codex_oracle title](assets/codex_oracle-title.png)

# 한국어

# codex_oracle

**개발자 프리뷰.** 이 프로젝트는 GitHub에서 로컬 Codex 플러그인을 직접 설치할 수
있는 개발자와 초기 Codex 사용자를 대상으로 합니다.

`codex_oracle`은 Codex app 안에서 GPT 5.5 모델과 `Pro 확장`을 이용해 계획,
설계, 코드 검토, 의사결정 피드백을 받기 위한 Codex 플러그인입니다.

이 프로젝트는 [steipete/oracle](https://github.com/steipete/oracle)의 유용한 아이디어에서
많은 힌트를 얻었습니다. Oracle이 보여준 "선별한 작업 맥락을 다른 모델에게 넘겨
검토받는 흐름"에 감사와 존중을 표합니다.

## 필요한 것

- Codex app
- `@Chrome` 플러그인 (`chrome@openai-bundled`)
- ChatGPT에 로그인된 사용자의 Chrome 세션
- GPT 5.5와 `Pro 확장`을 사용할 수 있는 계정 상태

프리뷰 배포와 라이브 검증은 `codex_oracle@personal`을 기준으로 합니다.

## 지원 환경

- Windows: `bin/codex_oracle.exe`
- macOS Apple Silicon: `bin/codex_oracle_darwin_arm64`
- macOS Intel: `bin/codex_oracle_darwin_amd64`

Windows `.exe`는 macOS 네이티브 바이너리가 아닙니다. macOS 설치에는 위 Darwin
바이너리 중 아키텍처에 맞는 파일을 사용해야 합니다.

macOS 바이너리는 서명/공증되지 않았습니다. 조직 정책이나 Gatekeeper가 차단하면
사용자가 직접 신뢰 여부를 확인해야 하며, 설치 에이전트가 보안 정책을 우회하면 안 됩니다.
이 개발 환경에는 실제 macOS 장비가 없어 macOS 지원은 cross-compile, MCP 템플릿,
Mach-O dry-run 검증까지만 완료되어 있습니다.

## 무엇을 하나요?

- Codex 작업 내용을 정리해 ChatGPT 웹 대화로 넘길 프롬프트를 만듭니다.
- `@Chrome`이 사용자의 로그인된 Chrome에서 새 ChatGPT 대화를 열도록 돕습니다.
- ChatGPT의 답변을 Codex 세션으로 다시 가져와 검토 자료로 사용합니다.

기본 라이브 경로는 `consult_prepare` -> `@Chrome` -> `consult_finalize`입니다. 레거시
direct-CDP live `consult`는 프리뷰 배포에서 기본 비활성화되어 있으며, 로컬 진단용으로만
명시적으로 켤 수 있습니다.

사용자가 `codex_oracle`에 프롬프트와 파일을 넣어 호출하면, 그 handoff를 ChatGPT에
제출하는 것은 이미 승인된 것으로 봅니다. 별도의 전송 확인을 다시 묻지 않고 `@Chrome`을
통해 진행할 수 있습니다. 단, 자격 증명 입력, CAPTCHA, 사람 확인, 잘못된 도메인, 예상 밖
민감 파일은 중단 조건입니다.

라이브 검토에는 작은 파일셋과 집중된 질문을 권장합니다. `dryRun:true`로 파일 전달과
bundle 크기를 먼저 확인하세요. ChatGPT 웹 입력창이 큰 inline prompt를 받지 못하는 경우
`codex_oracle`은 pasted-text attachment 방식과 짧은 제출 문구를 handoff에 제공합니다.

설치와 에이전트용 절차는 [docs/AGENT_INSTALL.md](docs/AGENT_INSTALL.md)를 보세요.

## 현재 검증 상태

GitHub 개발자 프리뷰 기준으로 Windows 스모크와 Chrome-assisted live 검증을 완료했습니다.
남은 사항은 macOS 실제 장비 미검증, unsigned/not notarized 바이너리, ChatGPT Web UI
변경 가능성 같은 비차단 프리뷰 리스크입니다.

---

# English

# codex_oracle

**Developer Preview.** This project is intended for developers and early Codex users who are
comfortable installing local Codex plugins from GitHub.

`codex_oracle` is a Codex plugin for using GPT 5.5 with `Pro 확장` from the Codex app
to get planning, design, code review, and decision-making feedback.

This project is inspired by [steipete/oracle](https://github.com/steipete/oracle). Thanks to Oracle
for showing a practical workflow: gather the right project context, send it to another capable
model, and bring the review back into the coding session.

## Requirements

- Codex app
- `@Chrome` plugin (`chrome@openai-bundled`)
- A user Chrome session already signed into ChatGPT
- Account access to GPT 5.5 and `Pro 확장`

Preview release use and live verification are based on `codex_oracle@personal`.

## Supported Platforms

- Windows: `bin/codex_oracle.exe`
- macOS Apple Silicon: `bin/codex_oracle_darwin_arm64`
- macOS Intel: `bin/codex_oracle_darwin_amd64`

The Windows `.exe` is not a native macOS binary. macOS installs must use the matching Darwin
binary above.

The macOS binaries are not signed or notarized. If Gatekeeper or an organization policy blocks
execution, the user must make the trust decision directly. Installation agents should not bypass
security policy.
This development environment does not include physical macOS hardware, so macOS support has been
implemented and dry-run verified through cross-compiled binaries, MCP templates, and Mach-O checks.

## What does it do?

- Prepares a focused prompt from the current Codex work.
- Lets `@Chrome` open a fresh ChatGPT web conversation in the user's signed-in Chrome session.
- Brings the ChatGPT answer back into Codex as review feedback.

The default live path is `consult_prepare` -> `@Chrome` -> `consult_finalize`. Legacy direct-CDP
live `consult` is disabled by default in the preview release and is only for explicit local
diagnostics.

When a user invokes `codex_oracle` with a prompt and files, submitting that prepared handoff to
ChatGPT is considered authorized. The agent can continue through `@Chrome` without asking for a
second send confirmation. It still stops for credential entry, CAPTCHA, human verification, wrong
domains, or unexpected sensitive files.

Live review works best with a focused question and a small file set. Run `dryRun:true` first to
check file resolution and bundle size. If ChatGPT Web rejects a large inline prompt, `codex_oracle`
returns a pasted-text attachment submission mode and a short composer prompt in the handoff.

For installation and agent-facing steps, see [docs/AGENT_INSTALL.md](docs/AGENT_INSTALL.md).

## Current Validation Status

For the GitHub developer-preview target, Windows smoke checks and Chrome-assisted live validation
have been completed. Remaining items are non-blocking preview risks: no physical macOS test yet,
unsigned/not-notarized binaries, and possible ChatGPT Web UI drift.
