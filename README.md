![codex_oracle 픽셀아트 배너](assets/codex_oracle-title.png)

# codex_oracle

Codex의 작업 맥락을 **GPT-5.6 Sol + Pro** 웹 세션에 전달하고, 계획·설계·코드 검토
피드백을 다시 Codex로 가져오는 로컬 플러그인입니다.

[![Developer Preview](https://img.shields.io/badge/status-developer_preview-f3a6ca)](RELEASE_NOTES.md)
[![Release v0.1.2](https://img.shields.io/badge/release-v0.1.2-315BFF)](RELEASE_NOTES.md)
[![Windows verified](https://img.shields.io/badge/Windows-live_verified-2f80ed)](RELEASE_NOTES.md#검증-결과)
[![macOS dry-run](https://img.shields.io/badge/macOS-dry--run_only-8a78d6)](RELEASE_NOTES.md#알려진-제한)
[![MIT License](https://img.shields.io/badge/license-MIT-3aa675)](LICENSE)

**한국어** · [English](README.en.md) · [빠른 설치](#빠른-설치) · [작동 방식](#작동-방식) ·
[지원 범위](#지원-범위) · [보안](#보안)

> **개발자 프리뷰:** Codex app에서 GitHub 플러그인을 직접 설치하고 검증할 수 있는
> 개발자와 초기 사용자를 대상으로 합니다.

## 무엇을 하나요?

- 현재 Codex 작업과 선택한 파일을 집중된 검토 프롬프트로 만듭니다.
- 외부 `@Chrome` 플러그인이 로그인된 사용자 Chrome에서 새 ChatGPT 대화를 엽니다.
- GPT-5.6 Sol의 `Pro` 응답을 회수해 Codex 작업의 계획·설계·리뷰 자료로 사용합니다.

## 작동 방식

```text
Codex 작업 + 선택 파일
          │
          ▼
  codex_oracle 준비
          │  workspaceRoot 안에서만 파일 묶음 생성
          ▼
 @Chrome → ChatGPT Web → GPT-5.6 Sol + Pro
          │
          ▼
  consult_finalize → Codex 피드백
```

기본 라이브 경로는 `consult_prepare` → `@Chrome` → `consult_finalize`입니다.
`codex_oracle`은 Chrome을 내장하거나 로그인 정보를 읽지 않습니다.

## 필요한 것

- Codex app
- `chrome@openai-bundled` 플러그인
- ChatGPT에 로그인된 사용자의 Chrome 세션
- GPT-5.6 Sol과 `Pro`를 사용할 수 있는 계정
- Windows x64, macOS Apple Silicon 또는 macOS Intel

## 빠른 설치

Codex 에이전트에게 다음과 같이 요청하는 방법을 권장합니다.

> `Burntgogi/codex_oracle`의 `v0.1.2` 개발자 프리뷰를 설치해 주세요. 먼저 체크섬과
> 대상 경로를 확인하고 dry-run 결과를 보여 준 뒤, 내 승인을 받아 개인 marketplace에
> 설치하세요. `codex_oracle@personal`과 `chrome@openai-bundled`가 활성화됐는지 검증하고
> 새 작업에서 작은 dry-run으로 첫 호출을 점검하세요.

직접 설치할 때는 움직이는 `main` 대신 릴리스 태그를 사용합니다.

```powershell
git clone --branch v0.1.2 --depth 1 https://github.com/Burntgogi/codex_oracle.git
cd codex_oracle
.\scripts\install-personal.ps1
# 출력 경로를 확인하고 승인한 뒤:
.\scripts\install-personal.ps1 -Apply
codex plugin add codex_oracle@personal
.\scripts\verify-installed.ps1
```

macOS에서는 `sh scripts/install-personal.sh`로 dry-run을 확인한 다음 승인 후
`sh scripts/install-personal.sh --apply`를 사용합니다. 전체 Windows/macOS 절차와 업데이트
방법은 [에이전트 설치 가이드](docs/AGENT_INSTALL.md)를 확인하세요.

## 첫 사용

Codex에서 `codex_oracle`을 호출하며 집중된 질문과 필요한 파일만 지정합니다.

```text
codex_oracle로 이 구현 계획을 GPT-5.6 Sol + Pro에 전달해 핵심 블로커와 개선안을 검토해 줘.
먼저 dry-run으로 파일 범위를 확인하고, @Chrome을 통해 새 ChatGPT 대화에서 실행한 뒤
결과를 현재 Codex 작업으로 가져와 줘.
```

파일을 전달할 때는 활성 Codex 작업 영역을 `workspaceRoot`로 지정합니다. 큰 파일 묶음보다
구조 설명과 검토에 필요한 파일 몇 개를 선택하는 편이 안정적입니다.

## 지원 범위

| 환경 | 파일 | 검증 상태 |
| --- | --- | --- |
| Windows x64 | `bin/codex_oracle.exe` | 설치·MCP 스모크·Chrome live E2E 완료 |
| macOS Apple Silicon | `bin/codex_oracle_darwin_arm64` | cross-compile·Mach-O·dry-run 완료 |
| macOS Intel | `bin/codex_oracle_darwin_amd64` | cross-compile·Mach-O·dry-run 완료 |

macOS는 실제 장비에서 실행하지 않았습니다. Windows 바이너리는 서명되지 않았고 macOS
바이너리는 서명·공증되지 않았으므로 OS 또는 조직 정책의 신뢰 확인이 필요할 수 있습니다.

## 검증 상태

`v0.1.2`는 Go 테스트와 정적 검사, 세 플랫폼 재현 빌드, 체크섬, 패키지 스모크,
Windows 설치본 검증, `GPT-5.6 Sol + Pro` Chrome-assisted live E2E를 통과한 개발자
프리뷰입니다. 자세한 변경과 제한은 [릴리즈 노트](RELEASE_NOTES.md)를 확인하세요.

## 보안

- `OPENAI_API_KEY`, Chrome 쿠키 또는 로그인 자격 증명을 요구하거나 저장하지 않습니다.
- 선택 파일은 명시한 `workspaceRoot` 안에서만 해석하고 경로·링크가 바뀌면 닫힌 상태로 실패합니다.
- 로그인, CAPTCHA, 사람 확인, 잘못된 도메인 또는 예상 밖 민감 파일이 나타나면 중단합니다.
- 공개 이슈에 토큰, 개인 프롬프트, 비공개 저장소 내용 또는 사용자 파일을 첨부하지 마세요.

취약점 신고와 정확한 신뢰 경계는 [SECURITY.md](SECURITY.md)를 확인하세요.

## 이전 0.1 계열 릴리스

`0.1.0-preview`와 `0.1.1-preview`는 이전 개발자 프리뷰 기록으로 유지합니다. 새 설치에는
현재 검증된 `v0.1.2` 태그를 사용하세요.

## 감사

이 프로젝트는 [steipete/oracle](https://github.com/steipete/oracle)이 보여 준 “필요한 작업
맥락을 선별해 다른 모델에 검토받고 결과를 다시 가져오는 흐름”에서 많은 힌트를 얻었습니다.
유용한 아이디어와 공개 작업을 제공한 유지관리자와 기여자께 감사드립니다.

## 라이선스

[MIT License](LICENSE)
