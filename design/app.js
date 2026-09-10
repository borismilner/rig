import {DEFAULTS, tokens, apply, toToml, contrast, separation, optimiseHues} from './theme.js';

/* ══ state ══════════════════════════════════════════════════════════════ */
const clone = o => JSON.parse(JSON.stringify(o));
let theme = clone(DEFAULTS);
let mode  = 'dark';
let applying = false;

const FACES = {
  display:['Fraunces','Instrument Serif','Bricolage Grotesque','Newsreader','IBM Plex Sans'],
  ui:['Inter Tight','Schibsted Grotesk','Manrope','IBM Plex Sans','Bricolage Grotesque'],
  mono:['JetBrains Mono','IBM Plex Mono','Martian Mono'],
};

/* ══ apply, and keep applying if anything else flips the theme ══════════ */
function render(){
  applying = true;
  apply(theme, mode, document.documentElement);
  document.documentElement.dataset.motion = theme.motion ? '1' : '0';
  applying = false;
  paintPalette();
  const ex = document.getElementById('export');
  if (ex) ex.value = toToml(theme, mode);
}
/* the audit harness sets data-theme directly rather than clicking, so the
   engine has to notice an outside change or every reading is of one theme */
new MutationObserver(muts=>{
  if (applying) return;
  for (const m of muts) if (m.attributeName === 'data-theme'){
    const t = document.documentElement.getAttribute('data-theme');
    if (t && t !== mode){ mode = t; syncThemeButtons(); render(); }
  }
}).observe(document.documentElement, {attributes:true, attributeFilter:['data-theme']});

function syncThemeButtons(){
  document.getElementById('t-dark').setAttribute('aria-pressed', String(mode==='dark'));
  document.getElementById('t-light').setAttribute('aria-pressed', String(mode==='light'));
}
function setMode(t){
  const go=()=>{ mode=t; syncThemeButtons(); render(); };
  document.startViewTransition ? document.startViewTransition(go) : go();
}
document.getElementById('t-dark').onclick=()=>setMode('dark');
document.getElementById('t-light').onclick=()=>setMode('light');

/* ══ the lab ════════════════════════════════════════════════════════════ */
const CONTROLS = [
  ['Faces', [
    {k:'faces.display', t:'select', label:'Display', opts:FACES.display},
    {k:'faces.ui',      t:'select', label:'UI',      opts:FACES.ui},
    {k:'faces.mono',    t:'select', label:'Mono',    opts:FACES.mono},
  ]],
  ['Type', [
    {k:'type.base',       label:'Base', min:13, max:20, step:.5, unit:'px'},
    {k:'type.scale',      label:'Scale', min:1.1, max:1.5, step:.01},
    {k:'type.lineHeight', label:'Line', min:1.3, max:1.8, step:.01},
    {k:'type.dispTight',  label:'Display tracking', min:-.05, max:.02, step:.002, unit:'em'},
    {k:'type.uiTight',    label:'UI tracking', min:-.03, max:.03, step:.002, unit:'em'},
  ]],
  ['Shape', [
    {k:'shape.radius',  label:'Radius',  min:0, max:24, step:1, unit:'px'},
    {k:'shape.density', label:'Density', min:.75, max:1.4, step:.05},
    {k:'shape.gut',     label:'Gutter',  min:1, max:3, step:.1, unit:'rem'},
  ]],
  ['Hues', [
    {k:'hues.rotate',      label:'Rotate',    min:0, max:360, step:1, unit:'°'},
    {k:'hues.dark.L',      label:'Dark L',    min:.55, max:.95, step:.005},
    {k:'hues.dark.C',      label:'Dark C',    min:.02, max:.18, step:.005},
    {k:'hues.light.L',     label:'Light L',   min:.25, max:.7,  step:.005},
    {k:'hues.light.C',     label:'Light C',   min:.02, max:.2,  step:.005},
  ]],
  ['Surfaces', [
    {k:'surfaces.hue',        label:'Neutral hue', min:0, max:360, step:1, unit:'°'},
    {k:'surfaces.chroma',     label:'Neutral C',   min:0, max:.06, step:.002},
    {k:'surfaces.dark.bg',    label:'Dark ground', min:.10, max:.35, step:.005},
    {k:'surfaces.dark.panel', label:'Dark panel',  min:.14, max:.45, step:.005},
    {k:'surfaces.dark.fg',    label:'Dark text',   min:.80, max:1,   step:.005},
    {k:'surfaces.light.bg',   label:'Light ground',min:.88, max:1,   step:.004},
    {k:'surfaces.light.glow', label:'Light glow',  min:.85, max:1,   step:.004},
  ]],
  ['Motion', [
    {k:'motion', t:'check', label:'Animate'},
  ]],
];
const get = (o,p) => p.split('.').reduce((a,k)=>a[k], o);
const set = (o,p,v) => { const ks=p.split('.'); const last=ks.pop();
  ks.reduce((a,k)=>a[k], o)[last] = v; };

function buildLab(){
  const body = document.getElementById('labbody');
  body.innerHTML = CONTROLS.map(([group, items]) => `
    <fieldset><legend>${group}</legend>
    ${items.map(c=>{
      const v = get(theme, c.k);
      if (c.t==='select') return `<div class="ctl"><label for="c-${c.k}">${c.label}</label>
        <select id="c-${c.k}" data-k="${c.k}" data-t="select">
        ${c.opts.map(o=>`<option ${o===v?'selected':''}>${o}</option>`).join('')}</select>
        <output></output></div>`;
      if (c.t==='check') return `<div class="ctl"><label for="c-${c.k}">${c.label}</label>
        <input type="checkbox" id="c-${c.k}" data-k="${c.k}" data-t="check" ${v?'checked':''}>
        <output></output></div>`;
      return `<div class="ctl"><label for="c-${c.k}">${c.label}</label>
        <input type="range" id="c-${c.k}" data-k="${c.k}" min="${c.min}" max="${c.max}"
          step="${c.step}" value="${v}"><output id="o-${c.k}">${v}${c.unit||''}</output></div>`;
    }).join('')}</fieldset>`).join('');

  body.addEventListener('input', e=>{
    const el = e.target, k = el.dataset.k; if(!k) return;
    const t = el.dataset.t;
    const v = t==='select' ? el.value : t==='check' ? el.checked : parseFloat(el.value);
    set(theme, k, v);
    const out = document.getElementById('o-'+k);
    if (out){
      const c = CONTROLS.flatMap(g=>g[1]).find(x=>x.k===k);
      out.textContent = (typeof v==='number' ? (Math.abs(v)<1&&v!==0 ? v.toFixed(3) : v) : v)
        + (c?.unit||'');
    }
    render();
  });
}
document.getElementById('t-lab').onclick=()=>document.getElementById('lab').classList.toggle('open');
document.getElementById('labclose').onclick=()=>document.getElementById('lab').classList.remove('open');
document.getElementById('copytoml').onclick=async e=>{
  await navigator.clipboard.writeText(document.getElementById('export').value).catch(()=>{});
  e.target.textContent='Copied'; setTimeout(()=>e.target.textContent='Copy',1400);
};
document.getElementById('reset').onclick=()=>{ theme=clone(DEFAULTS); buildLab(); render(); };
document.getElementById('optimise').onclick=e=>{
  const before=separation(theme.hues.members.map(m=>tokv('h-'+m.name))).worst;
  optimiseHues(theme, mode);
  render();
  const after=separation(theme.hues.members.map(m=>tokv('h-'+m.name))).worst;
  e.target.textContent=`ΔE ${before.toFixed(1)} → ${after.toFixed(1)}`;
  setTimeout(()=>e.target.textContent='Optimise separation', 2600);
};

/* ══ the projection diagram ═════════════════════════════════════════════ */
const SURFACES=['CLI','MCP','HTTP','window','tray','toast','palette','cron','URL'];
(function(){
  const g=document.getElementById('dg'), cx=220, cy=182, R=140;
  let s=`<circle class="ring" cx="${cx}" cy="${cy}" r="36"/>
         <circle class="ring" cx="${cx}" cy="${cy}" r="36" style="animation-delay:1.7s"/>`;
  SURFACES.forEach((n,i)=>{
    const a=(-90+i*(360/SURFACES.length))*Math.PI/180;
    const x=cx+Math.cos(a)*R, y=cy+Math.sin(a)*R;
    const d=`M${cx+Math.cos(a)*48} ${cy+Math.sin(a)*48} L${x} ${y}`;
    s+=`<path class="wire" d="${d}"/>
        <path class="flow" d="${d}" style="animation-delay:${(i*.31).toFixed(2)}s"/>
        <rect class="node" x="${x-28}" y="${y-11.5}" width="56" height="23" rx="7"/>
        <text x="${x}" y="${y+4}" text-anchor="middle">${n}</text>`;
  });
  s+=`<circle class="core" cx="${cx}" cy="${cy}" r="44"/>
      <text class="big" x="${cx}" y="${cy-1}" text-anchor="middle">rig</text>
      <text x="${cx}" y="${cy+15}" text-anchor="middle">registry</text>`;
  g.innerHTML=s;
})();

/* ══ the window ═════════════════════════════════════════════════════════ */
const APPS={
 shelf:{hue:'steel',ver:'1.2.0',a:'412 MB',b:'migrated 19',ms:12,title:'Index',cov:'coverage: partial',
  cols:['Collection','Items','Last run','Size','Health'],
  rows:[['library','4,182','2h ago','318 MB',96],['study','1,204','2h ago','61 MB',88],
        ['archive','17,900','6d ago','33 MB',41],['inbox','12','4m ago','2 MB',100]],
  acts:['Stats','Reindex'],
  log:[['14:22:03','shelf','index  scanned 1,204 files'],['14:22:03','graft','run    nightly-audit started'],
       ['14:22:04','shelf','index  done in 812ms']]},
 graft:{hue:'sage',ver:'0.4.1',a:'3 runs',b:'queue 1',ms:4,title:'Runs',cov:'coverage: partial',
  cols:['Assignment','State','Started','Frames','Progress'],
  rows:[['nightly-audit','running','18m ago','41,208',62],['spec-sweep','done','2h ago','9,110',100],
        ['tidy-logbook','failed','1d ago','812',37],['rehearse-drill','queued','-','0',4]],
  acts:['Timeline','Run'],
  log:[['14:19:41','graft','run    nightly-audit started'],['14:21:02','graft','frame  41,208 appended'],
       ['14:22:03','graft','tok    124.6k in, 8.1k out']]},
 dispatch:{hue:'indigo',ver:'2.0.0',a:'11 open',b:'2 overdue',ms:9,title:'Assignments',cov:'coverage: full',
  cols:['Assignment','Owner','Due','Age','Done'],
  rows:[['rig spec pass','me','today','4h',72],['grabbit revival','me','Fri','30d',12],
        ['shelf pilot','me','next week','2d',5],['romsort','me','-','9d',2]],
  acts:['Board','New'],
  log:[['14:20:55','dispatch','health 3 checks missed'],['14:21:10','dispatch','state  degraded'],
       ['14:22:00','dispatch','retry  in 8s']]},
 archi:{hue:'lilac',ver:'0.9.2',a:'237 elems',b:'4 views',ms:6,title:'Model',cov:'coverage: partial',
  cols:['View','Elements','Edges','Changed','Coverage'],
  rows:[['context','18','24','2h ago',100],['containers','61','88','2h ago',94],
        ['components','142','311','6d ago',63],['deployment','16','19','21d ago',28]],
  acts:['Canvas','Validate'],
  log:[['14:18:02','archi','model  upsert_element x12'],['14:18:04','archi','check  0 dangling edges'],
       ['14:22:01','archi','idle']]},
 snapper:{hue:'teal',ver:'0.3.0',a:'stopped',b:'-',ms:0,title:'Captures',cov:'coverage: partial',
  cols:['Capture','Kind','When','Size','Annotated'],
  rows:[['tray-now','region','4m ago','213 KB',3],['plan-diff','window','2h ago','1.1 MB',100],
        ['desktop','full','2h ago','2.4 MB',3],['rail-states','region','1d ago','88 KB',100]],
  acts:['Open folder','Capture'],
  log:[['12:45:02','snapper','exit   stopped by user'],['12:44:58','snapper','write  desktop.png'],
       ['12:44:51','snapper','start  region capture']]},
 nudge:{hue:null,ver:'1.0.0',a:'3 watches',b:'0 firing',ms:2,title:'Watches',cov:'coverage: full',
  cols:['Path','Trigger','Last fire','Fires','Health'],
  rows:[['~/me/inbox','any write','4m ago','128',100],['~/dl','new file','1h ago','44',100],
        ['~/me/study','md change','3d ago','9',82],['~/tmp','stale 7d','-','0',60]],
  acts:['Log','Add watch'],
  log:[['14:18:31','nudge','watch  ~/me/inbox fired'],['14:18:31','nudge','claude 1 assignment queued'],
       ['14:22:02','nudge','idle']]},
};
const ORDER=['shelf','graft','dispatch','archi','snapper','nudge'];
const LABEL={ok:'idle',degraded:'3 health fails',failing:'exit 1',stopped:'stopped'};
const pane=document.getElementById('pane'), rail=document.getElementById('rail'),
      notch=document.getElementById('notch');
function renderPane(k){
  const a=APPS[k];
  pane.innerHTML=`<div class="gp-h"><h4>${a.title}</h4><span class="chip hue">${a.cov}</span>
    <span class="acts">${a.acts.map((t,i)=>
      `<button class="btn${i===a.acts.length-1?' primary':''}">${t}</button>`).join('')}</span></div>
    <table class="tbl"><thead><tr>${a.cols.map(c=>`<th>${c}</th>`).join('')}</tr></thead><tbody>
    ${a.rows.map(r=>`<tr><td>${r[0]}</td><td class="m">${r[1]}</td><td class="m">${r[2]}</td>
      <td class="m">${r[3]}</td><td><span class="bar"><i style="width:${r[4]}%"></i></span></td></tr>`).join('')}
    </tbody></table><div class="tail">${a.log.map(l=>
      `<div><span class="ts">${l[0]}</span><span class="pg">${l[1]}</span><span>${l[2]}</span></div>`
    ).join('')}</div>`;
}
function select(k){
  const a=APPS[k];
  const go=()=>{
    rail.querySelectorAll('.mark').forEach(b=>b.setAttribute('aria-current',String(b.dataset.app===k)));
    // a program never owns an alarm hue, and a program with no identity hue
    // leaves the shell achromatic - which is what the shell is anyway
    document.documentElement.style.setProperty('--hue', a.hue?`var(--h-${a.hue})`:'var(--fg)');
    document.documentElement.style.setProperty('--ohue', a.hue?`var(--o-${a.hue})`:'var(--fg)');
    document.getElementById('ctx-name').textContent=k;
    document.getElementById('ctx-ver').textContent=a.ver;
    document.getElementById('ctx-a').textContent=a.a;
    document.getElementById('ctx-b').textContent=a.b;
    document.getElementById('strip-r').textContent=a.ms?`${k} ${a.ms} ms`:`${k} stopped`;
    renderPane(k);
    notch.style.translate=`0 ${36+ORDER.indexOf(k)*44}px`;
  };
  document.startViewTransition ? document.startViewTransition(go) : go();
}
rail.addEventListener('click',e=>{const b=e.target.closest('.mark'); if(b) select(b.dataset.app)});

/* ══ rail cases ═════════════════════════════════════════════════════════ */
const CASES=[
 {cap:'<b>All well.</b> No colour anywhere but the current program&rsquo;s notch.',
  m:[['sh','ok',1],['gr','ok',0],['ar','ok',0],['nu','ok',0]]},
 {cap:'<b>Degraded.</b> One amber pip, and nothing moves.',
  m:[['sh','ok',1],['gr','degraded',0],['ar','ok',0],['nu','ok',0]]},
 {cap:'<b>Failing, and one stopped.</b> The red pip breathes; a stopped program keeps its '+
      'place and goes dashed, so the rail never re-orders under your hand.',
  m:[['sh','ok',1],['gr','failing',0],['ar','stopped',0],['nu','ok',0]]}];
document.getElementById('railrow').innerHTML=CASES.map(c=>`<div>
  <div class="railcase"><div class="rail"><span class="brand">R</span>
  ${c.m.map(([t,s,cur])=>`<span class="mark" data-state="${s}" ${cur?'aria-current="true"':''}>
    <span class="ico">${t}</span>${s==='degraded'||s==='failing'?'<i class="pip"></i>':''}</span>`).join('')}
  <i class="notch" style="translate:0 ${36+c.m.findIndex(x=>x[2])*44}px"></i>
  </div></div><span class="cap">${c.cap}</span></div>`).join('');

/* ══ tray ═══════════════════════════════════════════════════════════════ */
function menu(st, banner){
  const rows=ORDER.map(k=>{const s=st[k]||'ok';
    return `<div class="tm-row"><i class="g ${s}"></i><span class="nm">${k}</span>
    <span class="st">${s==='ok'?(APPS[k].ms?APPS[k].ms+'ms':'idle'):LABEL[s]}</span></div>`}).join('');
  const bad=Object.values(st).filter(s=>s==='degraded'||s==='failing').length;
  return `<div class="tm-head"><span class="lg">R</span><span class="t">rig</span>
    <span class="s">${6-bad} mounted &middot; ${bad} degraded</span></div>
    ${banner?`<div class="tm-banner"><i class="pip"></i><span>${banner}</span></div>`:''}
    ${rows}<div class="tm-sep"></div><div class="tm-lbl">Run</div>
    <div class="tm-act">shelf reindex<span class="k">7d</span></div>
    <div class="tm-act">graft run nightly-audit</div>
    <div class="needsyou"><div class="tm-sep"></div><div class="tm-lbl">Needs you</div>
      <div class="tm-act">dispatch: restart budget spent<span class="k">quarantined</span></div>
      <div class="tm-act">Open the crash panel</div></div>
    <div class="tm-sep"></div>
    <div class="tm-act">Open the window<span class="k">Ctrl Space</span></div>
    <div class="tm-act">Notifications<span class="k">3</span></div>
    <div class="tm-act">Do not disturb</div>`;
}
document.getElementById('traypop').innerHTML=menu({snapper:'stopped'});
document.getElementById('trayscene').innerHTML=`
 <div><div class="traymenu">${menu({snapper:'stopped'})}</div>
  <span class="cap"><b>At rest.</b> Not one saturated pixel: six hollow ticks, a stopped program
  still holding its place, and no &ldquo;needs you&rdquo; section at all - the CSS hides it with
  <code>:has()</code> when no row is in trouble.</span></div>
 <div><div class="traymenu">${menu({graft:'degraded',dispatch:'failing',snapper:'stopped'},
  '<b>rigd is restarting</b> after an upgrade. Back in about 2 s; the window and this menu keep '+
  'working, and nothing was refused.')}</div>
  <span class="cap"><b>Something wants you.</b> A section appears that is not there at rest, and
  the lifecycle notice is drawn by the window process - which survives a daemon restart precisely
  so this banner can exist.</span></div>`;

/* ══ toasts ═════════════════════════════════════════════════════════════ */
const T={
 info:{src:'shelf',t:'Index rebuilt',b:'4,182 items in 812 ms. 3 files skipped as unreadable.',acts:['Show skipped','Open shelf']},
 success:{src:'graft',t:'nightly-audit finished',b:'18 minutes, 0 findings above medium. Report written.',acts:['Read report']},
 warn:{src:'rigd',t:'graft failed 3 health checks',b:'Degraded. Two more and the restart budget starts.',acts:['Logs','Restart now']},
 error:{src:'dispatch',t:'Exited 1 during migration 020',b:'Rolled back. The database is untouched and the last 200 log lines are kept.',acts:['Crash panel','Copy trace id']},
 progress:{src:'grabbit',t:'Downloading 3 of 8',b:'42.1 MB/s &middot; about 4 minutes left',bar:true,acts:[]}};
// severity asks for a MEANING, never for a colour
const TONE={info:'steel',success:'sage',warn:'amber',error:'rust',progress:'teal'};
function build(kind){
  const d=T[kind], el=document.createElement('div');
  const dwell=Math.min(20, 10+5*Math.ceil(d.b.length/46));
  el.className='toast '+kind;
  el.style.setProperty('--tone',`var(--h-${TONE[kind]})`);
  el.style.setProperty('--otone',`var(--o-${TONE[kind]})`);
  el.style.setProperty('--dwell', dwell+'s');
  el.innerHTML=`<div class="th"><span class="badge">${kind}</span><span class="src">${d.src}</span>
    <button class="x" aria-label="Dismiss">&times;</button></div>
    <p class="tt">${d.t}</p><p class="tb">${d.b}</p>
    ${d.bar?'<div class="prog"><i></i></div>':''}
    ${d.acts.length?`<div class="row">${d.acts.map(a=>`<button class="btn">${a}</button>`).join('')}</div>`:''}
    <i class="dwell"></i>`;
  return el;
}
const deck=document.getElementById('deck');
function dismiss(el){
  if (el.dataset.going) return;
  el.dataset.going='1';
  el.classList.add('leaving');
  el.addEventListener('animationend',()=>el.remove(),{once:true});
  setTimeout(()=>el.remove(), 400);   // belt and braces if the animation is off
}
function fire(kind){
  // newest first in the DOM, and the deck runs top-down, so an unread notice
  // is never underneath one you have already seen
  const el=build(kind); deck.prepend(el);
  while(deck.children.length>5) deck.lastElementChild.remove();
  const dw=el.querySelector('.dwell');
  el.addEventListener('mouseenter',()=>dw.style.animationPlayState='paused');
  el.addEventListener('mouseleave',()=>dw.style.animationPlayState='running');
  dw.addEventListener('animationend',()=>dismiss(el));
  el.querySelector('.x').addEventListener('click',()=>dismiss(el));
}
// Esc dismisses the top of the deck, the way every notification centre does
addEventListener('keydown',e=>{
  if(e.key==='Escape' && deck.firstElementChild && !document.getElementById('lab').classList.contains('open'))
    dismiss(deck.firstElementChild);
});
document.querySelectorAll('[data-fire]').forEach(b=>b.onclick=()=>fire(b.dataset.fire));
document.getElementById('fire-all').onclick=()=>
  ['info','success','warn','error','progress'].forEach((k,i)=>setTimeout(()=>fire(k),i*170));
['error','warn','progress','success','info'].forEach((k,i)=>setTimeout(()=>fire(k),250+i*110));
const st=build('success');
st.querySelector('.x').addEventListener('click',()=>dismiss(st));
document.getElementById('stagetoast').appendChild(st);

/* ══ palette, measured from the tokens actually painted ═════════════════ */
const USE={rust:'error · failing',amber:'warn · degraded',sage:'success · ok',
           teal:'progress · snapper',steel:'info · shelf',indigo:'dispatch',lilac:'archi'};
const GROUNDS=['bg','bg-2','panel','glow'];
const tokv=n=>getComputedStyle(document.documentElement).getPropertyValue('--'+n).trim();
function paintPalette(){
  const ms=theme.hues.members;
  document.getElementById('ring-n').textContent=ms.length;
  document.getElementById('swatches').innerHTML=ms.map(m=>`
    <div class="sw" style="--c:var(--h-${m.name})"><div class="band">${m.name}</div>
    <div class="meta"><span>${USE[m.name]||''}</span><b>${
      contrast(tokv('h-'+m.name), tokv('glow')).toFixed(2)}:1</b></div>
    <div class="meta"><span class="role">${m.role==='-'?'identity':m.role}${
      m.identity?'':' · reserved'}</span><span class="role">${m.angle}°</span></div></div>`).join('');

  const rows=[['fg',4.5],['fg-dim',4.5],['fg-faint',4.5],['border',3],['border-2',3],
    ...ms.map(m=>['h-'+m.name,4.5])];
  let worst=Infinity, fails=0;
  document.getElementById('ratiobody').innerHTML=rows.map(([n,need])=>{
    const cells=GROUNDS.map(g=>{
      const r=contrast(tokv(n), tokv(g)), ok=r>=need;
      if(!ok) fails++;
      worst=Math.min(worst, r/need*4.5);
      return `<td class="m ${ok?'pass':'bad'}">${r.toFixed(2)}${ok?'':' ✗'}</td>`;
    }).join('');
    return `<tr><td class="m">--${n}</td>${cells}<td class="m">${need.toFixed(1)}</td></tr>`;
  }).join('');

  // the second question: can two of them be told apart
  const hexes=ms.map(m=>tokv('h-'+m.name));
  const sep=separation(hexes);
  const w=worst.toFixed(2), sw=sep.worst.toFixed(1);
  document.getElementById('hero-ratio').textContent=w;
  document.getElementById('hero-sep').textContent=sw;
  document.getElementById('q-contrast').textContent=w+':1';
  document.getElementById('q-sep').textContent=sw;
  document.getElementById('q-pair').textContent=
    `${ms[sep.pair[0]].name} / ${ms[sep.pair[1]].name}`;

  const v=document.getElementById('verdict');
  v.classList.toggle('failing', fails>0);
  document.getElementById('v-worst').textContent=w;
  document.getElementById('v-sep').textContent=sw;
  document.getElementById('v-count').textContent=
    fails ? `${fails} cell${fails>1?'s':''} failing` : 'every token passes, both themes';
}

/* ══ go ═════════════════════════════════════════════════════════════════ */
buildLab();
render();
select('shelf');
