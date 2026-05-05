from pathlib import Path
import re

path = Path('/home/wcubuntu/ytb2bili-test/internal/chain_task/handlers/down_load_video.go')
text = path.read_text(encoding='utf-8')
pattern = r'''if cookiesPath != "" \{\n\s*command = append\(command, "--cookies", cookiesPath\)\n\s*t\.App\.Logger\.Infof\(".*?%s", cookiesPath\)\n\s*\}'''
replacement = '''if cookiesPath != "" {
		command = append(command, "--cookies", cookiesPath)
		t.App.Logger.Infof("🍪 使用 Cookies 文件: %s", cookiesPath)
	} else {
		command = append(command, "--cookies-from-browser", "chrome")
		t.App.Logger.Info("🍪 未找到导出的 Cookies 文件，尝试从 Chrome 读取 Cookies")
	}'''
new_text, count = re.subn(pattern, replacement, text, count=1, flags=re.S)
if count != 1:
    raise SystemExit('cookie block not found')
path.write_text(new_text, encoding='utf-8')
