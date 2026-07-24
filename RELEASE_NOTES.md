# codex_oracle 릴리즈 노트

[한국어](RELEASE_NOTES.md) · [English](RELEASE_NOTES.en.md)

## 0.1.2-preview

`v0.1.2`는 Codex 작업 맥락을 외부 `@Chrome` 플러그인을 통해 ChatGPT Web의
**GPT-5.6 Sol + Pro**로 전달하고 검토 결과를 회수하는 개발자 프리뷰입니다.

### 주요 변경

- GitHub 시작 페이지를 제품 중심 구조로 재구성하고 한국어·영어 README를 분리했습니다.
- 한국어·영어 릴리즈 노트를 추가하고 이전 `0.1.x` 프리뷰 기록을 보존했습니다.
- 기본 웹 대상을 GPT-5.6 Sol + `Pro`로 고정하고 `gpt-5.5-pro`는 입력 호환 별칭으로만 유지합니다.
- `consult`와 `consult_prepare`에 명시적 `workspaceRoot` 경계를 적용했습니다.
- 외부 디렉터리·글롭 기반을 순회 전에 거부하고, 파일 스냅샷을 루트 핸들에 묶어 경로나
  링크가 바뀌면 닫힌 상태로 실패하도록 강화했습니다.
- Windows amd64, macOS amd64, macOS arm64 바이너리와 SHA-256 체크섬을 다시 만들었습니다.

### 필요한 환경

- Codex app
- `chrome@openai-bundled`
- ChatGPT에 로그인된 사용자의 Chrome 세션
- GPT-5.6 Sol과 `Pro`에 접근 가능한 계정

### 검증 결과

- Go 1.26.4 테스트, vet, 포맷 검사
- Windows amd64와 Darwin amd64/arm64 CGO 비활성화·trimpath 빌드
- 릴리즈 패키지와 설치본 MCP 스모크
- `codex_oracle@personal` Windows 설치·캐시 일치 확인
- GPT-5.6 Sol + Pro Chrome-assisted live E2E와 `consult_finalize` 완료
- macOS arm64/x86_64 MCP 템플릿, Mach-O 형식, dry-run 확인

### 알려진 제한

- 실제 macOS 장비에서는 실행하지 않았습니다.
- Windows 바이너리는 서명되지 않았고 macOS 바이너리는 서명·공증되지 않았습니다.
- ChatGPT Web UI, 계정 모델 권한, 로그인 상태, CAPTCHA와 Cloudflare는 외부 의존성입니다.
- `@Chrome`은 필수 동반 플러그인이며 `codex_oracle`에 내장되지 않습니다.

## 이전 0.1 계열 릴리스

### 0.1.1-preview

- GPT-5.6 Sol + Pro 웹 핸드오프와 모델·Pro 모드 닫힌 상태 검증을 도입했습니다.
- `workspaceRoot`와 루트 고정 파일 스냅샷을 도입하기 전의 개발자 프리뷰입니다.

### 0.1.0-preview

- Windows/macOS Go 바이너리, 개인 marketplace 설치 스크립트, `@Chrome` 보조 핸드오프의
  초기 개발자 프리뷰입니다.
