/*
Liquid Charts by Ojieame · https://liquid.ojieame.design · nathan@ojieame.design

Liquid Charts
Copyright (c) 2026 NATEPAY, LLC. This file is source-available under the Liquid Charts
Source-Available Licence 1.0, not under MIT: https://liquid.ojieame.design/docs/licence

Free to use and change in anything you make, commercial products and client work included, as
long as the Liquid credit stays visible where the charts show it and this notice stays. You may
not redistribute or sell it as a chart library, component library, UI kit, template, registry
or other developer tool.

The notices below, and any later in this file, are kept word for word. The work each one
names keeps its own licence: the MIT notices below apply only to portions derived from the
projects they name, and a font keeps its font licence.

Liquid Charts adaptation. Original foundation: Evil Charts by Gurbinder.
Upstream: ecbd6a5db7b25f9a7e070c02b2ef728c378eeb5c
MIT License

Copyright (c) 2026 Gurbinder

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

Includes shadcn-derived primitives:
MIT License

Copyright (c) 2023 shadcn

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/
/** A glass mark's simulated liquid surface (`ChartOpticalMark.field`): `rows` rows of `columns`
 * floats. The first `depth` rows are heights (px from the level) across the mark, wall to wall,
 * from its front face to its back; then the coat left on the glass above the level; then the cloud's
 * flow (px), whether it moves, and whether the mark has a surface at all. `meniscus` is the px the
 * liquid climbs the walls. */
export const OPTICAL_SURFACE = { columns: 24, depth: 6, rows: 8, meniscus: 1.8 } as const;
const NX = OPTICAL_SURFACE.columns,
  NZ = OPTICAL_SURFACE.depth;

/** Authored studio/Fresnel/liquid-coverage response from the approved glass engine.
 * Geometry is supplied by exact native shape distance fields, never a bbox color ramp. */
export const opticalVertexSource = `#version 300 es
in vec2 position;
void main(){gl_Position=vec4(position,0.,1.);}`;
export const opticalFragmentSource = `#version 300 es
precision highp float;
uniform sampler2D uField;
uniform sampler2D uPalette;
uniform vec2 uSize;
uniform float uRatio;
uniform vec4 uFieldRect;
// A mask drawn larger or smaller than it was built (a growing mark) scales its distances.
uniform float uFieldGain;
uniform vec4 uBounds;
uniform float uRange;
uniform vec4 uColor;
uniform float uHasPalette;
uniform float uColorDirection;
uniform float uTone;
uniform float uGlow;
uniform float uTheme;
uniform float uDepth;
uniform float uBevel;
uniform float uShell;
uniform float uFill;
uniform int uOrigin;
uniform vec4 uAngular;
uniform vec2 uAngularCenter;
uniform int uKind;
uniform float uDepthAxis;
uniform float uDepthReverse;
uniform int uShapeKind;
uniform vec4 uShape;
uniform vec2 uAngles;
uniform float uOpacity;
uniform vec3 uFocus;
uniform float uHatch;
uniform vec2 uPattern;
uniform vec4 uMarks;
uniform float uSeed;
// 1 when a sector takes its tip light along its outer arc (a rose slice), 0 at its sweep's end.
uniform float uTip;
// The mark's simulated liquid surface, four floats to a vector (see OPTICAL_SURFACE).
uniform vec4 uLiquidSurface[${(NX * OPTICAL_SURFACE.rows) / 4}];
// 1: an exact shape, with no mask. The field a mask would hold is evaluated here instead, on the
// texel grid it would cover (uGrid: its origin in px, then texels per px), so the glass matches.
uniform int uExact;
uniform vec3 uGrid;
// 1: the mask holds distance alone (RG); its coverage follows from the distance.
uniform int uFieldFormat;
// The exact shape. A circle: centre, radius. A rect: centre, half size; then its corner radius. A
// sector: centre, inner radius plus its corner and outer radius less it; its four corner centres
// (outer, then inner, from the centre); its outer and inner arcs as start and sweep; its two sides'
// inward normals; then the corner, and 1 if it sweeps past half a turn.
uniform vec4 uExactShape[6];
out vec4 outputColour;
const float PI=3.14159265359;
// The angle round a ring, clockwise from 12 o'clock; a zero l.y (the row through the centre) is
// nudged below the centre line, as some GPUs return the opposite half-turn for atan(x,-0.).
float ringAngle(vec2 l){return atan(l.x,abs(l.y)<1e-4?-1e-4:-l.y);}
const float STEP=1.25;
// The cosine of glass's critical angle (index 1.5): refracted light crosses a wall at least this
// steeply.
const float CROSS=.745;
vec3 grey;
vec4 field(vec2 p){return texture(uField,clamp((p-uFieldRect.xy)/uFieldRect.zw,vec2(0.),vec2(1.)));}
float segmentDistance(vec2 p,vec2 a,vec2 b){vec2 ab=b-a;return length(p-a-ab*clamp(dot(p-a,ab)/max(dot(ab,ab),1e-9),0.,1.));}
// The exact shape's signed distance, px: a sector's rounded corners are its sides and arcs pulled
// in by the corner and pushed back out, which is what tangent rounds are.
float shapeDistance(vec2 p){
 vec4 s0=uExactShape[0];
 if(uShapeKind==3)return length(p-s0.xy)-s0.z;
 if(uShapeKind==1){float k=uExactShape[1].x;vec2 q=abs(p-s0.xy)-(s0.zw-k);return length(max(q,0.))+min(max(q.x,q.y),0.)-k;}
 vec4 s1=uExactShape[1],s2=uExactShape[2],s3=uExactShape[3],s4=uExactShape[4],s5=uExactShape[5];
 vec2 l=p-s0.xy;float r=length(l),a=ringAngle(l);
 float d=mod(a-s3.x,2.*PI)<=s3.y?abs(r-s0.w):min(length(l-s1.xy),length(l-s1.zw));
 if(s0.z>0.)d=min(d,mod(a-s3.z,2.*PI)<=s3.w?abs(r-s0.z):min(length(l-s2.xy),length(l-s2.zw)));
 d=min(d,min(segmentDistance(l,s2.xy,s1.xy),segmentDistance(l,s2.zw,s1.zw)));
 float h0=dot(s4.xy,l)-s5.x,h1=dot(s4.zw,l)-s5.x;
 bool inside=r>=s0.z&&r<=s0.w&&(s5.y>.5?max(h0,h1)>=0.:min(h0,h1)>=0.);
 return (inside?-d:d)-s5.x;
}
// Off its anti-aliased band, a mask texel holds the distance to the nearest texel centre of the
// other kind: on a curve about .4 texel more than the true distance outside and .25 to .33 less
// inside, and on an axis-aligned edge a whole number of texels. These are a built mask's values.
float latticeBias(float s,float band,float inside,float outside){
 return s<0.?mix(band,inside,smoothstep(.3,1.,-s)):mix(band,outside,smoothstep(.2,1.,s));}
// A mask texel at c: its distance (px), coverage, and depth down its column's run.
vec3 shapeTexel(vec2 c){
 float k=uGrid.z,s=shapeDistance(c)*k;
 if(uShapeKind==1){
  vec4 s0=uExactShape[0];vec2 q=abs(c-s0.xy)-(s0.zw-uExactShape[1].x);
  if(q.x<=0.||q.y<=0.){
   // Along its sides: the canvas covers a curved path's pixel in four sub-rows, each exactly
   // across, so a texel half inside a top or bottom side is inside from .125 texel out.
   float h=.5/k,across=clamp((min(c.x+h,s0.x+s0.z)-max(c.x-h,s0.x-s0.z))*k,0.,1.),rows=0.;
   for(int i=0;i<4;i++)rows+=step(abs(c.y+(float(i)-1.5)*.25/k-s0.y),s0.w);
   float a=min(255.,floor(64.*rows*across));
   if(a>0.&&a<255.)return vec3((.5-a/255.)/k,a/255.,0.);
   vec2 e=(abs(c-s0.xy)-s0.zw)*k;
   return vec3((a>=255.?-min(floor(-e.x),floor(.125-e.y))-1.:q.y<=0.?ceil(e.x):ceil(e.y-.125))/k,a/255.,0.);
  }
 }
 float band=uShapeKind==2?.02:.07,a=floor(255.*clamp(.5-band-s,0.,1.)+.5);
 float d=a>0.&&a<255.?.5-a/255.:s+(uShapeKind==2?latticeBias(s,band,-.33,.38):latticeBias(s,band,-.25,.43));
 float depth=0.;
 if(uShapeKind==3&&a>16.){
  vec4 s0=uExactShape[0];float reach=s0.z+(.5-band-16.5/255.)/k,dx=c.x-s0.x,h=sqrt(max(reach*reach-dx*dx,0.));
  float first=ceil((s0.y-h-uGrid.y)*k-.5),count=floor((s0.y+h-uGrid.y)*k-.5)-first+1.;
  depth=floor(clamp((floor((c.y-uGrid.y)*k)-first+.5)/max(1.,count),0.,1.)*255.+.5)/255.;
 }
 return vec3(d/k,a/255.,depth);
}
// The exact field at p, filtered between texel centres as a mask texture is; kept for the fragment.
vec2 shapeAt=vec2(-1e9);vec3 shapeHere;
vec3 shapeField(vec2 p){
 if(p==shapeAt)return shapeHere;
 vec2 t=(p-uGrid.xy)*uGrid.z-.5,i=floor(t),f=t-i,c=uGrid.xy+(i+.5)/uGrid.z,e=vec2(1./uGrid.z,0.);
 vec3 a=shapeTexel(c),row0=f.x>0.?mix(a,shapeTexel(c+e.xy),f.x):a,row1=row0;
 if(f.y>0.){vec3 b=shapeTexel(c+e.yx);row1=f.x>0.?mix(b,shapeTexel(c+e.xx),f.x):b;}
 shapeHere=mix(row0,row1,f.y);shapeHere.x=clamp(shapeHere.x,-uRange,uRange);shapeAt=p;
 return shapeHere;
}
float planeDistance(vec2 p){if(uExact==1)return shapeField(p).x;vec4 f=field(p);return uFieldGain*(((f.r*65280.+f.g*255.)/65535.-.5)*2.*uRange);}
// How much of this device pixel the outline covers: a box one device px wide across the edge, from
// an exact shape's own distance or the mask's, so the cut is as fine as a vector path's at any
// mask density.
float edgeCoverage(vec2 p){return clamp(.5-(uExact==1?shapeDistance(p):planeDistance(p))*uRatio,0.,1.);}
float fieldDepth(vec2 p){return uExact==1?shapeField(p).z:field(p).a;}
vec2 planeNormal(vec2 p){return normalize(vec2(planeDistance(p+vec2(.6,0.))-planeDistance(p-vec2(.6,0.)),planeDistance(p+vec2(0.,.6))-planeDistance(p-vec2(0.,.6)))+vec2(.00001));}
float shellAt(float d,float z){
 vec2 q=vec2(d+uBevel,abs(z+uDepth*.5)-(uDepth*.5-uBevel));
 return length(max(q,0.))+min(max(q.x,q.y),0.)-uBevel;
}
float shell(vec3 p,int unused){return shellAt(planeDistance(p.xy),p.z);}
vec3 normalAt(vec3 p,int id){const vec2 k=vec2(1.,-1.);float h=.35;
 return normalize(k.xyy*shell(p+k.xyy*h,id)+k.yyx*shell(p+k.yyx*h,id)+k.yxy*shell(p+k.yxy*h,id)+k.xxx*shell(p+k.xxx*h,id));}

vec4 pigment(vec2 p);
vec3 studio(vec3 r, float light) {
  float overhead = smoothstep(.4, .62, -r.y) * smoothstep(.85, .6, abs(r.x));
  float right = smoothstep(.48, .64, r.x) * smoothstep(.72, .38, abs(r.y));
  float left = smoothstep(.55, .72, -r.x) * smoothstep(.72, .38, abs(r.y));
  float front = smoothstep(.8, 1., r.z);
  float ground = smoothstep(.12, .5, r.y);
  float base = mix(.035, 1., light) * (1. - ground * mix(.7, .08, light));
  return vec3(base + overhead * mix(5.5, .5, light) + right * mix(2.8, .3, light)
              + left * mix(.9, .15, light) + front * .1);
}

float hash(vec3 p) {
  p = fract(p * .3183099 + .1);
  p *= 17.;
  return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}
float cloud(vec3 x) {
  vec3 i = floor(x), f = fract(x);
  f = f * f * (3. - 2. * f);
  return mix(mix(mix(hash(i), hash(i + vec3(1, 0, 0)), f.x),
                 mix(hash(i + vec3(0, 1, 0)), hash(i + vec3(1, 1, 0)), f.x), f.y),
             mix(mix(hash(i + vec3(0, 0, 1)), hash(i + vec3(1, 0, 1)), f.x),
                 mix(hash(i + vec3(0, 1, 1)), hash(i + vec3(1, 1, 1)), f.x), f.y), f.z);
}

const int NX=${NX},NZ=${NZ};
float cell(int k){return uLiquidSurface[k>>2][k&3];}
// The simulated surface at x (0 to NX - 1, wall to wall) in row r, px; and its extras
// (0 flow, 1 moving, 2 has a surface).
float fieldAt(float x,int r){
 float fx=clamp(x,0.,float(NX-1));int x0=int(floor(fx)),x1=min(x0+1,NX-1);
 return mix(cell(r*NX+x0),cell(r*NX+x1),fx-float(x0));
}
float fieldExtra(int k){return cell((NZ+1)*NX+k);}
// The meniscus: px the liquid climbs the walls, keeping the volume.
float wallRise(float s,float halfW){return ${OPTICAL_SURFACE.meniscus.toFixed(2)}*(exp(-(1.-abs(s))*halfW/2.2)-min(2.2/max(halfW,1.),1.));}
// Where p lies across the mark: s from -1 to 1 wall to wall, px from the middle to a wall, and px
// of liquid along the fill; a half width of 0 means the fill has no surface to simulate.
vec3 acrossOf(vec3 p){
 float h=max(1.,uBounds.w),w=max(1.,uBounds.z);
 if(uOrigin<=1){
  float level=uFill*(h+2.),halfW=w*.5,s=(p.x-(uBounds.x+halfW))/halfW;
  if(uShapeKind==3){float y=uOrigin==0?uBounds.y+h-level:uBounds.y+level;float dy=y-uShape.y;
   halfW=sqrt(max(uShape.z*uShape.z-dy*dy,1.));s=(p.x-uShape.x)/halfW;}
  return vec3(clamp(s,-1.,1.),halfW,level);
 }
 if(uOrigin<=3)return vec3(clamp(-(p.y-(uBounds.y+h*.5))/(h*.5),-1.,1.),h*.5,uFill*(w+2.));
 if(uOrigin==4&&uShapeKind==2){float r=length(p.xy-uAngularCenter);
  return vec3(clamp((r-uShape.z)/max(uShape.w,.5),-1.,1.),uShape.w,uFill*uAngular.y*r);}
 return vec3(0.);
}
// The surface's offset at p on the front face (px along the fill); 0 without a surface.
float surfaceWave(vec3 p){
 if(fieldExtra(2)<.5)return 0.;
 vec3 c=acrossOf(p);
 if(c.y==0.)return 0.;
 return (fieldAt((c.x+1.)*.5*float(NX-1),0)+wallRise(c.x,c.y))*clamp(c.z/6.,0.,1.);
}
float density3(vec3 q,int unused){
 bool horizontal=uOrigin==2||uOrigin==3;
 vec2 anchor=uOrigin==1?vec2(uBounds.x+uBounds.z*.5,uBounds.y):uOrigin==2?vec2(uBounds.x,uBounds.y+uBounds.w*.5)
  :uOrigin==3?vec2(uBounds.x+uBounds.z,uBounds.y+uBounds.w*.5):vec2(uBounds.x+uBounds.z*.5,uBounds.y+uBounds.w);
 // The cloud travels with the liquid while it pours, then settles back.
 vec2 grow=uOrigin==1?vec2(0.,1.):uOrigin==2?vec2(1.,0.):uOrigin==3?vec2(-1.,0.):vec2(0.,-1.);
 vec3 l=(q-vec3(anchor+grow*fieldExtra(0),0.))/(horizontal?vec3(150.,46.,48.):vec3(46.,150.,48.));l+=uSeed;
 return .78+.44*(cloud(l)*.75+cloud(l*2.1+3.7)*.25);}
vec2 liquidFront(vec3 p,int unused);
// How far the fill runs, px: the bar's length, or for a radial fill its outer less inner radius.
float fillSpan(){return uOrigin==5?max(1.,uAngular.y):(uOrigin==2||uOrigin==3)?max(1.,uBounds.z):max(1.,uBounds.w);}
float edgeLight(vec3 p,int unused){
 if(uShapeKind==2&&uTip>.5){
  // A sector lit as a bar growing outward: the outer arc (or the liquid's surface while it fills)
  // is the tip, the two radial sides are its long edges (the one facing the key light brighter)
  // and the inner arc is its anchored end.
  vec2 l=p.xy-uShape.xy;float r=length(l),span=max(abs(uAngles.y-uAngles.x),.0001);
  float direction=uAngles.y>=uAngles.x?1.:-1.;
  float around=mod((ringAngle(l)-uAngles.x)*direction+.1,2.*PI)-.1;
  float depthR=max(uShape.w*2.,1.),along=r-(uShape.z-uShape.w);
  float level=min(uFill*(depthR+2.),depthR),tip=max(level-along,0.),anchor=max(along,0.);
  vec2 key=normalize(vec2(.55,-.81));
  float f0=.55+.45*max(dot(-direction*vec2(cos(uAngles.x),sin(uAngles.x)),key),0.);
  float f1=.55+.45*max(dot(direction*vec2(cos(uAngles.y),sin(uAngles.y)),key),0.);
  float d0=max(around*r,0.),d1=max((span-around)*r,0.);
  float side=max(span*r*.15,.5),reach=22.+.26*depthR;
  return 2.3*exp(-tip/reach)+1.25*f0*exp(-d0/side)+1.25*f1*exp(-d1/side)+.55*exp(-anchor/(side*1.2));
 }
 if(uShapeKind==1){
  float fullWidth=max(1.,uShape.z*2.),fullHeight=max(1.,uShape.w);
  bool horizontal=uOrigin==2||uOrigin==3;
  float h=horizontal?fullWidth:fullHeight,halfSpan=horizontal?fullHeight*.5:uShape.z;
  float along=uShape.y-p.y,across=p.x-uShape.x;
  if(uOrigin==1)along=p.y-(uShape.y-fullHeight);
  if(uOrigin==2){along=p.x-(uShape.x-uShape.z);across=p.y-(uShape.y-fullHeight*.5);}
  if(uOrigin==3){along=uShape.x+uShape.z-p.x;across=p.y-(uShape.y-fullHeight*.5);}
  float tip=max(min(uFill*(h+2.)+surfaceWave(p),h)-along,0.),anchor=max(along,0.);
  float left=max(halfSpan+across,0.),right=max(halfSpan-across,0.);
  float reach=22.+.26*h,side=max(halfSpan*.3,.5);
  return 2.3*exp(-tip/reach)+(horizontal?1.1:.8)*exp(-left/side)+(horizontal?.8:1.1)*exp(-right/side)+.55*exp(-anchor/(side*1.2));
 }
 if(uShapeKind==2){
  vec2 l=p.xy-uShape.xy;float r=length(l),span=max(abs(uAngles.y-uAngles.x),.0001);
  float direction=uAngles.y>=uAngles.x?1.:-1.;float along=mod((ringAngle(l)-uAngles.x)*direction+.1,2.*PI)-.1;
  float tip=max((uFill*span-along)*r+surfaceWave(p),0.),anchor=max(along*r,0.);
  float outer=max(uShape.z+uShape.w-r,0.),inner=max(r-uShape.z+uShape.w,0.);
  float facing=.55+.45*max(dot(l/max(r,1.),normalize(vec2(.55,-.81))),0.);
  float reach=26.+.24*span*uShape.z,side=max(uShape.w*.75,.5);
  return 2.1*exp(-tip/reach)+1.1*facing*exp(-outer/side)+.7*exp(-inner/side)+.4*exp(-anchor/side);
 }
 float inside=max(-planeDistance(p.xy),0.);vec2 n=planeNormal(p.xy);
 float small=max(1.,min(uBounds.z,uBounds.w));
 float facing=.55+.45*max(dot(n,normalize(vec2(.55,-.81))),0.);
 float edge=(1.15+1.3*facing)*exp(-inside/(small*.17+2.));
 // Filling outward, the tip is the liquid's surface while it rises, then the outline itself.
 if(uOrigin==5)return edge+1.05*exp(-min(max(liquidFront(p,0).x,0.),inside)/(22.+.2*uAngular.y));
 float topDepth=fieldDepth(p.xy)*uBounds.w;
 return edge+1.05*exp(-topDepth/(22.+.2*uBounds.w));
}
vec2 liquidFront0(vec3 p);
vec2 liquidFront(vec3 p,int unused){vec2 f=liquidFront0(p);f.x+=surfaceWave(p);return f;}
vec2 liquidFront0(vec3 p){
 float h=max(1.,uBounds.w),w=max(1.,uBounds.z);
 // Full, the surface sits 2px past the tip (as in the approved renderer), so its meniscus never
 // lands on the tip edge as a hard line.
 if(uOrigin==1)return vec2(uBounds.y+uFill*(h+2.)-p.y,clamp((p.y-uBounds.y)/h,0.,1.));
 if(uOrigin==2)return vec2(uBounds.x+uFill*(w+2.)-p.x,clamp((p.x-uBounds.x)/w,0.,1.));
 if(uOrigin==3)return vec2(p.x-(uBounds.x+w)+uFill*(w+2.),clamp((uBounds.x+w-p.x)/w,0.,1.));
 if(uOrigin==5){float along=length(p.xy-uAngularCenter)-uAngular.x,span=max(uAngular.y,1.);return vec2(uFill*(span+2.)-along,clamp(along/span,0.,1.));}
 if(uOrigin==4){vec2 l=p.xy-uAngularCenter;float a=ringAngle(l);float along=mod((a-uAngular.x)*uAngular.z+2.*PI,2.*PI);float span=max(uAngular.y,.0001);return vec2((uFill*(span+.02)-along)*length(l),clamp(along/span,0.,1.));}
 return vec2(p.y-(uBounds.y+h)+uFill*(h+2.),clamp((uBounds.y+h-p.y)/h,0.,1.));
}

vec4 surface(vec3 p, vec3 rd, int id, float light, out float exitT) {
  exitT = 64. * STEP;
    vec3 n = normalAt(p, id);
    vec4 tint = vec4(pigment(p.xy).rgb, uTone);
    float tone = tint.a;
    vec3 colour = mix(mix(grey, tint.rgb, .1 * (1. - light)), tint.rgb, tone);
    // Walk into the solid for its coverage: path in the liquid, path in the bare wall, and the
    // liquid's surface where the walk crosses it (only visible while it fills).
    float inLiquid = 0., inShell = 0., meniscus = 0.;
    // The simulated surface along this ray. Straight on, every sample shares the ray's place
    // across the mark, so its height at each depth row, and the coat above it, are read once.
    float rows[NZ];
    float coat = 0.;
    vec3 c3 = acrossOf(p);
    if (fieldExtra(2) > .5 && c3.y > 0.) {
      float fx = (c3.x + 1.) * .5 * float(NX - 1);
      float fade = clamp(c3.z / 6., 0., 1.), rise = wallRise(c3.x, c3.y);
      for (int r = 0; r < NZ; r++) rows[r] = (fieldAt(fx, r) + rise) * fade;
      coat = fieldAt(fx, NZ) * fade;
    } else {
      for (int r = 0; r < NZ; r++) rows[r] = 0.;
    }
    // The ray runs straight in (rd is always (0, 0, -1)), so every sample shares p's distance.
    float across = planeDistance(p.xy);
    for (int k = 0; k < 64; k++) {
      vec3 q = p + rd * (float(k) + .5) * STEP;
      float d = shellAt(across, q.z);
      if (d > 0.) {
        exitT = (float(k) + .5) * STEP;
        break;
      }
      // Through the depth the surface is a band, not a line: front and back rows differ.
      float dz = clamp(-q.z / max(uDepth, 1.), 0., 1.) * float(NZ - 1);
      int r0 = int(floor(dz));
      float here = mix(rows[r0], rows[min(r0 + 1, NZ - 1)], dz - float(r0));
      vec2 front = liquidFront0(q);
      front.x += here;
      // The liquid fills the glass out to its sides; only the front and back walls hold it in. Seen
      // edge-on, a side wall shows the liquid behind it, as refraction shows it in a real glass,
      // not a band of bare glass (a straight walk would run down the wall). Light entering bends
      // to within glass's critical angle, so it is through the front wall in a few px.
      float crossed = max(min(q.z, -CROSS * (float(k) + .5) * STEP), -uDepth - q.z);
      float inside = smoothstep(-uShell + .8, -uShell - .8, crossed);
      float liquid = inside * smoothstep(-.8, .8, front.x);
      // The coat the liquid left on the glass above it: a thin, translucent film.
      float film = coat > .3 ? inside * (1. - liquid) * smoothstep(-.8, .8, front.x - here + coat) : 0.;
      inLiquid += STEP * (liquid + film * .07);
      // Only the glass wall itself scatters; wall behind the liquid is hidden by it.
      inShell += STEP * (1. - liquid) * step(-uShell - .8, d) * exp(-inLiquid / 3.);
      meniscus += STEP * exp(-front.x * front.x / 2.2) * step(crossed, -uShell);
    }
    // Edge light through the gel, with a faint cloudiness so no two solids are identical.
    float light3 = edgeLight(p, id) * mix(.86, 1.14, (density3(p, id) - .78) / .44);
    float entered = 1. - exp(-light3 * mix(1.15, 1.3, light));
    // Dark cards: light that has come far has lost all but the colour's deepest tone, so the core
    // is dark and saturated; where it enters it is pale and milky.
    float deepest = mix(2.4, 4.2, tone);
    vec3 night = pow(max(colour, vec3(.02)), vec3(mix(deepest, .34, entered)));
    night *= mix(mix(.4, .16, tone), 1., entered) * mix(.8, 1., tone);
    // Light cards stay high-key: the core is the pure, vivid colour and the light coming in
    // dilutes it toward white at the edges and the tip. Nothing goes muddy.
    vec3 vivid = pow(max(colour, vec3(.02)), vec3(mix(1.15, 1.3, tone)));
    vec3 day = mix(vivid, mix(colour, vec3(1.), mix(.8, .6, tone)), pow(entered, 1.8));
    vec3 body = mix(night, day, light);
    vec3 milk = mix(sqrt(colour), vec3(1.), .45) * mix(mix(.6, 1., light), 1., tone);
    body = mix(body, milk, (1. - exp(-max(inShell - 12., 0.) / 9.)) * .3);
    body += mix(sqrt(colour), vec3(1.), .5) * min(meniscus / 5., 1.) * .5;
    // Etched pattern, as in the raymarched renderer: frosted diagonal lanes from uPattern.y of the
    // fill direction to its end, sized to the mark's thickness, with a fine seam where it begins.
    float etched = 0.;
    if (uPattern.x > .5) {
      float position = liquidFront(p, id).y;
      float span = fillSpan();
      float along = position * span, from = uPattern.y * span;
      float region = smoothstep(from - .6, from + .6, along);
      float period = clamp(min(uBounds.z, uBounds.w) * .31, 5.5, 12.);
      float lane = abs(fract((p.x + p.y - uBounds.x - uBounds.y) / period) - .5) * period;
      float etch = (1. - smoothstep(period * .2 - .5, period * .2 + .5, lane)) * region;
      etch = max(etch, exp(-(along - from) * (along - from) / .9) * step(.5, from));
      etched = etch;
      vec3 frost = mix(mix(sqrt(colour), vec3(1.), .35), vec3(1.), light * .55);
      body = mix(body, mix(body, vec3(dot(body, vec3(.3333))), .22), region * .6);
      body = mix(body, frost, etch * mix(.42, .62, light));
    }

    // Glass reflects the studio by Schlick's Fresnel term; every state keeps this, so inactive
    // graphite still has its silver rims.
    vec3 V = -rd;
    float facing = max(dot(n, V), 0.);
    float fresnel = min(.045 + .955 * pow(1. - facing, 5.), .24);
    vec3 env = studio(reflect(rd, n), light);
    vec3 lit = min(body * (1. - fresnel) + env * fresnel * mix(.45, 1., light), vec3(1.));
    // On white, the bevel turns away from the room and reads as a fine darker edge, which is what
    // keeps a pale solid from dissolving into the card.
    lit = mix(lit, lit * vec3(.85), light * (1. - smoothstep(.45, .95, facing)));
    // Liquid is near-opaque; a bare wall is nearly clear face-on and only builds up where the
    // view grazes it, so an empty shell reads as an outline of glass.
    float liquidCover = 1. - exp(-inLiquid / 3.);
    float wallCover = (1. - exp(-inShell * inShell / 1100.)) * (1. - liquidCover);
    vec3 wall = min(milk * mix(.25, .9, light) + env * fresnel, vec3(1.));
    float glassAlpha = clamp(fresnel * mix(1.6, .55, light), 0., 1.) * (1. - liquidCover - wallCover);
    vec3 glass = mix(min(env, vec3(1.)), vec3(.74), light);
    float filled = uFill;
    glassAlpha *= mix(1., 1. - filled, light);
    // Where the ray only clips the very edge, the rim is thinner than a pixel and would speckle.
    glassAlpha *= smoothstep(.6, 3., exitT);
  vec4 result = vec4(lit * liquidCover + wall * wallCover + glass * glassAlpha,
                     liquidCover + wallCover + glassAlpha);
  // Etched lanes show in the empty glass too, as frost in the bare wall, so planned or projected
  // work that holds no liquid yet still reads as etched.
  float bare = etched * (1. - liquidCover) * smoothstep(.6, 3., exitT);
  if (bare > 0.) {
    vec3 frosted = mix(mix(sqrt(colour), vec3(1.), .15), mix(colour, vec3(.47), .4), light);
    float a = bare * mix(.34, .3, light);
    result.rgb = result.rgb * (1. - a) + frosted * a;
    result.a = result.a * (1. - a) + a;
  }
  // Engraved reference marks, as in the raymarched renderer: glowing grooves that take the
  // liquid's colour once it has passed them, and are pale frost in the empty glass before that.
  if (max(max(uMarks.x, uMarks.y), max(uMarks.z, uMarks.w)) >= 0.) {
    float span = fillSpan();
    float along = liquidFront(p, id).y * span, level = uFill * (span + 2.);
    float small = max(1., min(uBounds.z, uBounds.w));
    float rim = .72 + .28 * exp(-max(-planeDistance(p.xy), 0.) / max(small * .18, .5));
    for (int k = 0; k < 4; k++) {
      float at = k == 0 ? uMarks.x : k == 1 ? uMarks.y : k == 2 ? uMarks.z : uMarks.w;
      if (at < 0.) continue;
      // A share past 2 is a graduation: a short tick cut in from the near edge, not a ring; past 4,
      // a graduation ring. Either is a faint engraving, as in the raymarched renderer.
      float reach = 1.;
      bool graduated = at > 1.5;
      if (at > 3.5) {
        at -= 4.;
      } else if (at > 1.5) {
        at -= 2.;
        float side = (uOrigin == 2 || uOrigin == 3)
          ? (p.y - uBounds.y) / max(1., uBounds.w)
          : (p.x - uBounds.x) / max(1., uBounds.z);
        reach = 1. - smoothstep(.2, .3, side);
        if (reach <= 0.) continue;
      }
      float d = along - at * span;
      bool reached = level >= at * span - .5;
      if (graduated) {
        // A faint engraving: a soft groove a shade off the glass, no bright edge, no halo (the
        // raymarched renderer's ETCH, .45).
        float groove = exp(-d * d / .5) * reach * .45;
        vec3 cut = reached ? mix(mix(colour, vec3(1.), .4), colour * .8, light) : mix(vec3(.8), vec3(.5), light);
        float a = groove * (reached ? mix(.3, .32, light) : mix(.22, .2, light));
        result.rgb = result.rgb * (1. - a) + cut * a;
        result.a = result.a * (1. - a) + a;
        continue;
      }
      float core = exp(-d * d / .32) + .3 * exp(-d * d / 4.);
      float flank = exp(-(abs(d) - 1.35) * (abs(d) - 1.35) / .3) * reach;
      vec3 glow = reached ? mix(mix(sqrt(colour), vec3(1.), .55), vec3(1.), light * .6)
                          : mix(vec3(.88), vec3(.52), light);
      float a = clamp(core * rim, 0., 1.) * (reached ? mix(.9, .85, light) : mix(.7, .58, light)) * reach;
      // A passed mark scatters a little light into the liquid around it, so it reads as lit.
      float halo = reached ? exp(-d * d / 14.) * mix(.22, .08, light) * reach : 0.;
      result.rgb *= 1. - flank * mix(.18, .16, light) * result.a;
      result.rgb += mix(sqrt(colour), vec3(1.), .4) * halo * result.a;
      result.rgb = result.rgb * (1. - a) + glow * a;
      result.a = result.a * (1. - a) + a;
    }
  }
  return result;
}

vec4 pigment(vec2 p){float t=uColorDirection>.5?(p.y-uBounds.y)/max(uBounds.w,1.):(p.x-uBounds.x)/max(uBounds.z,1.);return uHasPalette>.5?texture(uPalette,vec2(clamp(t,0.,1.),.5)):uColor;}
vec4 glassAt(vec2 p){
 // An exact shape shows nothing past 52px: the glow ends at 46, the shadow 40 below a 10px drop.
 if(uExact==1&&shapeDistance(p)>52.)return vec4(0.);
 float light=step(.5,uTheme);float d=planeDistance(p);vec4 outp=vec4(0.);
 float coverage=edgeCoverage(p);
 if(coverage>0.){
  // A pixel the outline cuts is shaded from half a device px inside it: it takes the glass's own
  // colour and only its coverage falls off, never a rim of whatever lies past the edge.
  float inset=.5/uRatio;vec2 q=p;float e=d;
  if(d>-inset){q=p-planeNormal(p)*(d+inset);e=planeDistance(q);}
  float edge=clamp(e+uBevel,0.,uBevel);float z=-uBevel+sqrt(max(0.,uBevel*uBevel-edge*edge));
  float exitT;vec4 c=surface(vec3(q,z),vec3(0.,0.,-1.),0,light,exitT);
  outp=c*coverage;
 }
 // Glow and shadow follow the approved renderer: a pastel halo and a soft shadow on white, a
 // faint light on dark. Both end inside the mark's 48px field padding so nothing is cut square,
 // and small marks glow less so dense charts don't build a haze.
 float small=max(1.,min(uBounds.z,uBounds.w));
 float spread=min(mix(16.,28.,light),small*.45+6.);
 float outside=max(d,0.);
 float support=1.-smoothstep(30.,46.,outside);
 if(light>.5&&d>0.){
  float shadowSpread=min(12.,small*.3+2.);
  float shadowDistance=max(planeDistance(p-vec2(0.,min(10.,small*.25+2.))),0.);
  float shade=exp(-shadowDistance/shadowSpread)*(1.-smoothstep(24.,40.,shadowDistance))*min(uFill*1.5,1.)*.035*min(uGlow,1.)*(1.-outp.a);
  outp.a+=shade;
 }
 float strength=mix(.05,.1,light)*mix(.5,1.,smoothstep(8.,36.,small))*uGlow;
 float bloom=exp(-outside/spread)*support*uTone*min(uFill*1.5,1.)*strength;
 vec3 glow=mix(uColor.rgb,vec3(1.),light*.12);
 outp.rgb+=glow*bloom*(1.-outp.a);outp.a+=bloom*(1.-outp.a);
 return outp;
}
vec4 areaAt(vec2 p){
 vec4 f=field(p);float d=planeDistance(p),inside=f.b;float depth=f.a;
 vec3 colour=pigment(p).rgb;
 float dist=max(-d,0.);float top=1.-smoothstep(.15,.48,depth);
 bool horizontal=uDepthAxis>.5;
 float crossPosition=horizontal?p.y:p.x;float crossStart=horizontal?uBounds.y:uBounds.x;
 float crossSize=horizontal?uBounds.w:uBounds.z;float focusPosition=horizontal?uFocus.y:uFocus.x;
 vec2 surfaceFacing=horizontal?vec2(-1.,0.):vec2(0.,-1.);surfaceFacing*=uDepthReverse>.5?-1.:1.;
 float focus=uFocus.z*exp(-pow((crossPosition-focusPosition)/46.,2.));float pool=focus*exp(-dist/70.)*inside*top;
 float meniscus=exp(-dist/5.)*inside*top;
 float wallDistance=min(crossPosition-crossStart,crossStart+crossSize-crossPosition);
 float walls=mix(.12,1.,smoothstep(0.,min(40.,crossSize*.12),wallDistance));
 float rim=mix(.55,1.,smoothstep(0.,min(24.,crossSize*.1),wallDistance));
 float body=.85*pow(1.-depth,2.1)*inside;
 vec3 lit=mix(colour,mix(colour,vec3(1.),.38),meniscus*.65);lit=mix(lit,mix(colour,vec3(1.),.3),pool*.55);
 float a=max(max(body,pool*.6)*walls,meniscus*.92*rim);
 float bloom=exp(-pow(max(d,0.)/6.5,2.))*(1.-inside)*(.26+.4*focus)*rim*smoothstep(-.1,.3,dot(planeNormal(p),surfaceFacing));
 vec3 premultiplied=lit*a+colour*bloom*(1.-a);float alpha=a+bloom*(1.-a);
 if(uTheme>.5){float wash=.46*pow(1.-depth,1.65)*inside;float skin=meniscus*.56*rim;alpha=max(max(wash,pool*.32)*walls,skin);lit=mix(colour,vec3(.96),meniscus*.16);premultiplied=lit*alpha;}
 float lane=abs(fract((p.x+p.y)/18.)-.5)*18.;
 float hatch=(1.-smoothstep(.35,1.3,lane))*inside*smoothstep(4.,36.,dist)*pow(1.-depth,.7)*(.07+.1*focus)*walls*uHatch;
 premultiplied=mix(colour,vec3(1.),.55)*hatch+premultiplied*(1.-hatch);alpha=hatch+alpha*(1.-hatch);
 float front=liquidFront(vec3(p,0.),0).x;float reveal=smoothstep(-.6,.6,front);return vec4(premultiplied,alpha)*reveal;
}
void main(){
 vec2 p=vec2(gl_FragCoord.x/uRatio,uSize.y-gl_FragCoord.y/uRatio);
 grey=mix(vec3(.63),vec3(.8),step(.5,uTheme));
 vec4 result=uKind==1?areaAt(p):glassAt(p);
 float noise=fract(52.9829189*fract(dot(gl_FragCoord.xy,vec2(.06711056,.00583715))))-.5;
 outputColour=vec4(result.rgb+noise/255.*result.a,result.a)*uOpacity*pigment(p).a;
}
`;
