import pathlib
import re
import sys
import urllib.request
import zipfile
from urllib.parse import urljoin

# 开发期工具：下载参考二维码库 segno 到 work\pylibs，
# 用来逐模块校验自研的 Go 版二维码编码器。不参与最终程序。
INDEX = "https://pypi.tuna.tsinghua.edu.cn/simple/segno/"
dest = pathlib.Path(__file__).parent / "pylibs"
dest.mkdir(parents=True, exist_ok=True)

html = urllib.request.urlopen(INDEX, timeout=30).read().decode("utf-8", "replace")
urls = [u for u in re.findall(r'href="([^"]+\.whl[^"]*)"', html) if "py3-none-any" in u]
if not urls:
    sys.exit("没有找到 wheel")
url = urljoin(INDEX, urls[-1]).split("#")[0]
print("wheel:", url)
data = urllib.request.urlopen(url, timeout=120).read()
whl = dest / "segno.whl"
whl.write_bytes(data)
print("bytes:", len(data))
with zipfile.ZipFile(whl) as z:
    z.extractall(dest)
whl.unlink()
print("extracted ->", dest)
