import math, json
f=lambda v:f"{v:.1f}"
def centroid(ps):
    A=cx=cy=0
    for (x0,y0),(x1,y1) in zip(ps,ps[1:]+ps[:1]):
        c=x0*y1-x1*y0; A+=c; cx+=(x0+x1)*c; cy+=(y0+y1)*c
    A/=2; return cx/(6*A), cy/(6*A)
def fit(groups, key, R=33, lift=1.0):
    """Translate and scale every layer so the area centroid of `key` sits at the icon centre
    (lifted a touch, which reads as centred) and its farthest point is R from it."""
    cx,cy=centroid(groups[key]); m=max(math.hypot(x-cx,y-cy) for x,y in groups[key]); s=R/m
    T=lambda p:(54+(p[0]-cx)*s, 54-lift+(p[1]-cy)*s)
    return {k:[T(p) for p in v] for k,v in groups.items()}, T, s
path=lambda ps:"M"+" L".join(f(x)+","+f(y) for x,y in ps)+"Z"
o={}

# ---------- scallops ----------
def scallop(span0,span1,lobes,depth,R,hinge_w,round_base):
    hx,hy=0,0; pts=[]; N=360
    a0,a1=math.radians(span0),math.radians(span1)
    for i in range(N+1):
        t=a0+(a1-a0)*i/N; ph=(i/N)*lobes*math.pi
        r=R*(1+depth*abs(math.sin(ph)))
        pts.append((r*math.cos(t),r*math.sin(t)))
    # base: curve from the rim's last point round to the hinge and back, so the bottom is rounded
    base=[]
    e=pts[-1]; s=pts[0]
    for i in range(1,40):
        u=i/40
        # quadratic through hinge point below centre
        bx=(1-u)**2*e[0]+2*(1-u)*u*0+u**2*s[0]
        by=(1-u)**2*e[1]+2*(1-u)*u*(round_base)+u**2*s[1]
        base.append((bx,by))
    return pts+base, (a0,a1)
def ribs(a0,a1,lobes,R,inner=0.18):
    out=[]
    for k in range(1,lobes):
        t=a0+(a1-a0)*k/lobes
        out.append(((R*inner*math.cos(t),R*inner*math.sin(t)),(R*0.95*math.cos(t),R*0.95*math.sin(t))))
    return out
def ears(w,h,y):
    L=[(-w*0.25,y-h*0.6),(-w,y-h*0.3),(-w*1.02,y+h*0.5),(-w*0.3,y+h*0.6)]
    Rr=[(-x,yy) for x,yy in L][::-1]
    return L,Rr

# G2: round cockle, wide span, 7 broad lobes, rounded base
pts,(a0,a1)=scallop(190,350,7,0.05,40,0,14)
rb=ribs(a0,a1,7,40)
eL,eR=ears(16,9,6)
g={'body':pts,'eL':eL,'eR':eR}
for i,(p,q) in enumerate(rb): g[f'r{i}']=[p,q]
G,T,s=fit(g,'body',R=36.5)
o['g2Body']=path(G['body']); o['g2Ears']=path(G['eL'])+" "+path(G['eR'])
o['g2Ribs']=" ".join(f"M{f(G[f'r{i}'][0][0])},{f(G[f'r{i}'][0][1])} L{f(G[f'r{i}'][1][0])},{f(G[f'r{i}'][1][1])}" for i in range(len(rb)))
# alternate wedges for banding
wed=[]
for k in range(0,7,2):
    t0=a0+(a1-a0)*k/7; t1=a0+(a1-a0)*(k+1)/7
    w=[(0,0)]+[(41*math.cos(t0+(t1-t0)*j/20),41*math.sin(t0+(t1-t0)*j/20)) for j in range(21)]
    wed.append(path([T(p) for p in w]))
o['g2Bands']=" ".join(wed)

# G3: pebble scallop, rounder still, 11 shallow lobes, curved ribs, tiny ears
pts,(a0,a1)=scallop(182,358,11,0.03,40,0,18)
eL,eR=ears(12,7,8)
g={'body':pts,'eL':eL,'eR':eR}
G,T,s=fit(g,'body',R=36.5)
o['g3Body']=path(G['body']); o['g3Ears']=path(G['eL'])+" "+path(G['eR'])
cr=[]
for k in range(1,11):
    t=a0+(a1-a0)*k/11
    p0=T((7*math.cos(t),7*math.sin(t))); p2=T((38*math.cos(t),38*math.sin(t)))
    bend=0.10*(k-5.5)/5.5
    pm=T((24*math.cos(t+bend),24*math.sin(t+bend)))
    cr.append(f"M{f(p0[0])},{f(p0[1])} Q{f(pm[0])},{f(pm[1])} {f(p2[0])},{f(p2[1])}")
o['g3Ribs']=" ".join(cr)

# ---------- oysters ----------
def oyster(rx,ry,tear,wob,seed,n=300,tip_angle=225):
    ps=[]
    for i in range(n):
        t=2*math.pi*i/n
        w=1+wob*(0.55*math.sin(3*t+seed)+0.3*math.sin(6*t+2*seed)+0.18*math.sin(11*t+3*seed))
        tp=1-tear*max(0,math.cos(t-math.radians(tip_angle)))**1.5
        ps.append((rx*w*tp*math.cos(t),ry*w*tp*math.sin(t)))
    return ps
def rot(ps,deg):
    c,s=math.cos(math.radians(deg)),math.sin(math.radians(deg)); return [(x*c-y*s,x*s+y*c) for x,y in ps]
def inset(ps,k,dx=0,dy=0):
    cx,cy=centroid(ps); return [(cx+(x-cx)*k+dx,cy+(y-cy)*k+dy) for x,y in ps]

# H2: teardrop leaning, hinge upper-left; centred on its area centroid so the heavy rounded end
# doesn't pull the icon down-right; nacre takes most of the face
outer=rot(oyster(36,27,0.34,0.05,1.1),-40)
lip=inset(rot(oyster(36,27,0.34,0.045,1.9),-40),0.88)
inner=inset(rot(oyster(36,27,0.34,0.025,0.6),-40),0.72,1.2,1.0)
G,T,s=fit({'o':outer,'l':lip,'i':inner},'o',R=34)
o['h2Outer']=path(G['o']); o['h2Lip']=path(G['l']); o['h2Inner']=path(G['i'])
# highlight follows the long axis
hp=[T(p) for p in rot([(-14,-9),(-2,-14),(12,-12)],-40)]
o['h2Hi']=f"M{f(hp[0][0])},{f(hp[0][1])} Q{f(hp[1][0])},{f(hp[1][1])} {f(hp[2][0])},{f(hp[2][1])}"

# H3: cupped oyster, hinge at the top, a gentle lean; a pearl sits on the lighter side to counterweigh
outer=rot(oyster(28,33,0.30,0.06,2.4,tip_angle=270),-14)
lip=inset(rot(oyster(28,33,0.30,0.05,3.1,tip_angle=270),-14),0.87)
inner=inset(rot(oyster(28,33,0.30,0.03,0.9,tip_angle=270),-14),0.7,0,2)
G,T,s=fit({'o':outer,'l':lip,'i':inner},'o',R=34)
o['h3Outer']=path(G['o']); o['h3Lip']=path(G['l']); o['h3Inner']=path(G['i'])
# lean is to the right at the top, so the pearl sits low-left
px,py=T(rot([(-6,9)],-14)[0]); o['h3Pearl']=(round(px,1),round(py,1),round(6.2*s,1))
hp=[T(p) for p in rot([(-10,-12),(0,-17),(9,-13)],-14)]
o['h3Hi']=f"M{f(hp[0][0])},{f(hp[0][1])} Q{f(hp[1][0])},{f(hp[1][1])} {f(hp[2][0])},{f(hp[2][1])}"
json.dump(o,open('paths4.json','w')); print({k:(len(v) if isinstance(v,str) else v) for k,v in o.items()})
