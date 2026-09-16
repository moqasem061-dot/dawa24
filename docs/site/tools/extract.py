import subprocess, re, json, os, collections
os.chdir(os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', '..'))
files = subprocess.run(['git','ls-files'],capture_output=True,text=True,encoding='utf-8').stdout.split('\n')
files=[f for f in files if f]
skip = re.compile(r'^internal/ui/data/uploads/|/vendor/|^test/corpus/files|visual_baselines|\.(png|jpg|jpeg|webp|svg|ico|gif|woff2?)$')
out=[]
def lines(p):
    try: return open(p,encoding='utf-8',errors='ignore').read()
    except: return ''
for f in files:
    if skip.search(f): continue
    if f.endswith('_templ.go'): continue
    s=lines(f); n=s.count('\n')
    hint=''
    if f.endswith('.go'):
        m=re.findall(r'^((?://[^\n]*\n)+)(?:package|func|type|var|const)', s, re.M)
        if m: hint=' '.join(l.strip('/ ').strip() for l in m[0].splitlines())
        names=re.findall(r'^func (?:\([^)]*\) )?([A-Z]\w*)|^type ([A-Z]\w*)', s, re.M)
        nm=[a or b for a,b in names][:8]
        if f.endswith('_test.go'): nm=re.findall(r'^func (Test\w+)', s, re.M)[:6]
        hint=(hint[:260]+' | '+','.join(nm))
    elif f.endswith('.templ'):
        nm=re.findall(r'^templ (\w+)', s, re.M)[:8]
        c=re.findall(r'^//\s?(.*)', s, re.M)[:2]
        hint=' '.join(c)[:160]+' | '+','.join(nm)
    elif f.endswith('.sql'):
        c=[l.strip('- ').strip() for l in s.splitlines()[:6] if l.startswith('--')]
        hint=' '.join(c)[:200]
    elif f.endswith(('.css','.js')):
        m=re.search(r'/\*(.*?)\*/|^//(.*)', s, re.S|re.M)
        hint=(m.group(1) or m.group(2) or '').strip()[:200].replace('\n',' ') if m else ''
    elif f.endswith('.md'):
        m=re.search(r'^#\s+(.*)',s,re.M); hint=m.group(1) if m else ''
    out.append((f,n,hint.replace('\t',' ').replace('\n',' ')))
with open(os.environ.get('OUT', os.path.join(os.path.dirname(os.path.abspath(__file__)), 'hints.tsv')),'w',encoding='utf-8') as w:
    for f,n,h in out: w.write(f'{f}\t{n}\t{h}\n')
print(len(out))
