import json,re,math
p=json.load(open('paths4.json'))
R='/Users/ryder/Code/Nacre/android/app/src/main/res/drawable/'
def pts(d): return [tuple(map(float,m)) for m in re.findall(r'(-?[\d.]+),(-?[\d.]+)',d)]
def bbox(d):
    ps=pts(d); xs=[x for x,_ in ps]; ys=[y for _,y in ps]; return min(xs),min(ys),max(xs),max(ys)
def centroid(ps):
    A=cx=cy=0
    for (x0,y0),(x1,y1) in zip(ps,ps[1:]+ps[:1]):
        c=x0*y1-x1*y0; A+=c; cx+=(x0+x1)*c; cy+=(y0+y1)*c
    A/=2; return cx/(6*A),cy/(6*A)
# bigger pearl, kept clear of the lining's edge
px,py,pr=p['h3Pearl']; pr=8.6
inner=pts(p['h3Inner']); icx,icy=centroid(inner)
def clearance(x,y): return min(math.hypot(x-a,y-b) for a,b in inner)
while clearance(px,py)<pr+1.6:
    px+= (icx-px)*0.08; py+=(icy-py)*0.08
px,py=round(px,1),round(py,1)
print('pearl',px,py,pr,'clearance',round(clearance(px,py),1))

def lin(d,c0,c1,x1=0,y1=0,x2=1,y2=1):
    a,b,c,e=bbox(d); w,h=c-a,e-b
    return f'''    <path android:pathData="{d}">
        <aapt:attr name="android:fillColor">
            <gradient android:type="linear" android:startX="{a+w*x1:.1f}" android:startY="{b+h*y1:.1f}" android:endX="{a+w*x2:.1f}" android:endY="{b+h*y2:.1f}">
                <item android:offset="0" android:color="{c0}" />
                <item android:offset="1" android:color="{c1}" />
            </gradient>
        </aapt:attr>
    </path>'''
def rad(d,cx,cy,r,col,alpha):
    a,b,c,e=bbox(d); w,h=c-a,e-b
    return f'''    <path android:pathData="{d}">
        <aapt:attr name="android:fillColor">
            <gradient android:type="radial" android:centerX="{a+w*cx:.1f}" android:centerY="{b+h*cy:.1f}" android:gradientRadius="{r*(w+h)/2:.1f}">
                <item android:offset="0" android:color="#{alpha}{col}" />
                <item android:offset="1" android:color="#00{col}" />
            </gradient>
        </aapt:attr>
    </path>'''
circ=lambda x,y,r: f"M{x-r:.1f},{y:.1f}a{r:.1f},{r:.1f} 0 1,0 {2*r:.1f},0a{r:.1f},{r:.1f} 0 1,0 {-2*r:.1f},0z"
ell=lambda x,y,rx,ry: f"M{x-rx:.1f},{y:.1f}a{rx:.1f},{ry:.1f} 0 1,0 {2*rx:.1f},0a{rx:.1f},{ry:.1f} 0 1,0 {-2*rx:.1f},0z"
pearl_path=circ(px,py,pr)
fg=f'''<?xml version="1.0" encoding="utf-8"?>
<!-- Nacre: a cupped oyster, open, with its pearly lining and a pearl. Centred on the shell's area so its lean
     doesn't pull the icon off balance; the pearl sits low-left as a counterweight. -->
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:aapt="http://schemas.android.com/aapt"
    android:width="108dp" android:height="108dp" android:viewportWidth="108" android:viewportHeight="108">
  <group android:pivotX="54" android:pivotY="54" android:scaleX="0.95" android:scaleY="0.95">
{lin(p['h3Outer'],'#FFD6CEE4','#FF9A91B8')}
{lin(p['h3Lip'],'#FFF0EAF5','#FFB7AFCB')}
{lin(p['h3Inner'],'#FFFBF7FD','#FFE2EEF7')}
{rad(p['h3Inner'],.3,.3,.55,'F6BFD8','F2')}
{rad(p['h3Inner'],.72,.32,.45,'C6B4F4','F2')}
{rad(p['h3Inner'],.7,.75,.55,'8FDDE0','F2')}
{rad(p['h3Inner'],.28,.78,.4,'C4F0CF','E6')}
    <path android:pathData="{p['h3Hi']}" android:strokeColor="#BFFFFFFF" android:strokeWidth="2" android:strokeLineCap="round" />
    <path android:pathData="{pearl_path}">
        <aapt:attr name="android:fillColor">
            <gradient android:type="radial" android:centerX="{px-pr+0.76*pr:.1f}" android:centerY="{py-pr+0.68*pr:.1f}" android:gradientRadius="{1.4*pr:.1f}">
                <item android:offset="0" android:color="#FFFFFFFF" />
                <item android:offset="0.55" android:color="#FFEEE8F8" />
                <item android:offset="1" android:color="#FFA9D3DC" />
            </gradient>
        </aapt:attr>
    </path>
    <path android:pathData="{ell(px-pr*0.3,py-pr*0.35,pr*0.32,pr*0.22)}" android:fillColor="#E6FFFFFF" />
  </group>
</vector>
'''
open(R+'ic_launcher_foreground.xml','w').write(fg)
mono=f'''<?xml version="1.0" encoding="utf-8"?>
<!-- Themed icon: the shell, its hollow lining and the pearl. -->
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="108dp" android:height="108dp" android:viewportWidth="108" android:viewportHeight="108">
  <group android:pivotX="54" android:pivotY="54" android:scaleX="0.95" android:scaleY="0.95">
    <path android:fillType="evenOdd" android:fillColor="#FF000000" android:pathData="{p['h3Outer']} {p['h3Inner']}" />
    <path android:fillColor="#FF000000" android:pathData="{pearl_path}" />
  </group>
</vector>
'''
open(R+'ic_launcher_monochrome.xml','w').write(mono)
