def lin(c):
    c/=255.0
    return c/12.92 if c<=0.03928 else ((c+0.055)/1.055)**2.4
def L(h):
    h=h.lstrip('#'); r,g,b=(int(h[i:i+2],16) for i in (0,2,4))
    return 0.2126*lin(r)+0.7152*lin(g)+0.0722*lin(b)
def cr(a,b):
    la,lb=L(a),L(b)
    hi,lo=max(la,lb),min(la,lb)
    return (hi+0.05)/(lo+0.05)

DARK = dict(bg="#161b26", bg2="#1f252e", panel="#242b34", glow="#212a3d",
            fg="#dae1e8", fgdim="#9fb0bf", fgfaint="#94a3b0",
            border="#657589", border2="#7d8fa3")
LIGHT= dict(bg="#eef1f5", bg2="#e3e8ee", panel="#ffffff", glow="#dfe7f2",
            fg="#171d25", fgdim="#4a5563", fgfaint="#5b6675",
            border="#b9c2cd", border2="#8e9aa8")
HUES = {"steel":("#2f577f","#8dadce"), "clay":("#7f432f","#ce9d8d"),
        "lilac":("#5d2f7f","#bf9dd8"), "sand":("#75622b","#cebe8d"),
        "sage":("#297029","#8dce8d"), "teal":("#27685e","#8dcec3")}

for name,T in (("LIGHT",LIGHT),("DARK",DARK)):
    print(f"\n=== {name} : foreground tokens on each surface ===")
    print(f"{'token':10} {'bg':>7} {'bg2':>7} {'panel':>7} {'glow':>7}  need")
    for t in ("fg","fgdim","fgfaint","border","border2"):
        need = "3.0" if t.startswith("border") else "4.5"
        vals=[cr(T[t],T[s]) for s in ("bg","bg2","panel","glow")]
        flag="" if all(v>=float(need) for v in vals) else "   <-- FAIL"
        print(f"{t:10} " + " ".join(f"{v:7.2f}" for v in vals) + f"  {need}{flag}")

for name,T,idx in (("LIGHT",LIGHT,0),("DARK",DARK,1)):
    print(f"\n=== {name} : the six plugin hues as text ===")
    print(f"{'hue':8} {'bg':>7} {'bg2':>7} {'panel':>7} {'glow':>7}  6% tint of itself")
    for h,(lv,dv) in HUES.items():
        c = (lv,dv)[idx]
        vals=[cr(c,T[s]) for s in ("bg","bg2","panel","glow")]
        # 6% tint of the hue over panel
        def mix(a,b,p):
            a=a.lstrip('#'); b=b.lstrip('#')
            return "#"+"".join(f"{round(int(a[i:i+2],16)*p+int(b[i:i+2],16)*(1-p)):02x}" for i in (0,2,4))
        tint = mix(c, T["panel"], 0.06)
        tv = cr(c, tint)
        flag="" if all(v>=4.5 for v in vals) and tv>=4.5 else "   <-- CHECK"
        print(f"{h:8} " + " ".join(f"{v:7.2f}" for v in vals) + f"  {tv:7.2f}{flag}")
