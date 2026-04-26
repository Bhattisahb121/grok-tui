# grok-tui

Grok 웹 채팅을 터미널에서 사용할 수 있는 TUI 클라이언트.

브라우저 인스턴스 없이 순수 HTTP/HTTPS로 grok.com 웹 API를 직접 호출합니다.

## 주요 기능

- **브라우저 없이 동작** — Go `net/http` + TLS 설정으로 직접 API 호출
- **Cloudflare 우회** — 브라우저와 동일한 TLS fingerprint (cipher suites, HTTP/2)
- **실시간 스트리밍** — NDJSON 스트림을 파싱하여 토큰 단위로 실시간 출력
- **마크다운 렌더링** — Grok 응답을 터미널에서 마크다운으로 렌더링
- **MFA/Challenge 대응** — Cloudflare, MFA, 세션 만료 등을 TUI에서 직접 안내하고 해결
- **대화 관리** — 연속 대화, 새 대화 시작, 모델 변경 지원

## 설치

```bash
go install github.com/Bhattisahb121/grok-tui/cmd/grok-tui@latest
```

또는 소스에서 빌드:

```bash
git clone https://github.com/Bhattisahb121/grok-tui.git
cd grok-tui
make build
```

## 설정

### 1. 설정 파일 생성

```bash
grok-tui init
```

이 명령은 `~/.config/grok-tui/config.json` 파일을 생성합니다.

### 2. Cookie 추출

1. 브라우저에서 [grok.com](https://grok.com)에 로그인
2. DevTools 열기 (F12) → Network 탭
3. Grok에 메시지를 보내기
4. `/rest/app-chat/conversations/new` 요청을 찾기
5. Request Headers에서 `Cookie` 값을 복사

### 3. 설정 파일 편집

```json
{
  "cookie": "x-anonuserid=...; x-challenge=...; x-signature=...; sso=...; sso-rw=...",
  "statsig_id": "",
  "model": "grok-3"
}
```

`cookie` 필드에 복사한 Cookie 값을 붙여넣으세요.

## 사용법

```bash
grok-tui
```

### TUI 명령어

| 명령어 | 설명 |
|--------|------|
| `/new`, `/reset` | 새 대화 시작 |
| `/model [name]` | 모델 확인/변경 (`grok-3`, `grok-3-mini`, `grok-2`) |
| `/cookie <값>` | Cookie 업데이트 (세션 만료/MFA 후) |
| `/check` | 연결 상태 확인 |
| `/help` | 도움말 |
| `/quit`, `/exit` | 종료 |

### 단축키

| 키 | 동작 |
|----|------|
| `Enter` | 메시지 전송 |
| `Ctrl+D` | 줄바꿈 삽입 |
| `Ctrl+C` | 스트리밍 취소 / 종료 |

## MFA / Cloudflare 처리

Cloudflare 챌린지나 MFA가 필요한 경우 TUI에 자동으로 안내 메시지가 표시됩니다:

1. 브라우저에서 grok.com을 열어 챌린지/MFA를 통과
2. 새 Cookie 값을 복사
3. TUI에서 `/cookie <새 쿠키 값>` 입력

Cookie는 자동으로 설정 파일에 저장되어 다음 실행 시에도 유지됩니다.

## 환경 변수

| 변수 | 설명 |
|------|------|
| `GROK_TUI_CONFIG` | 설정 파일 경로 (기본: `~/.config/grok-tui/config.json`) |

## 요구 사항

- Go 1.23+
- grok.com 계정 (로그인하여 Cookie 추출 필요)

## 라이선스

MIT
