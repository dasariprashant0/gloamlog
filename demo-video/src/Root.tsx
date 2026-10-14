import React from "react";
import { Composition } from "remotion";
import { Demo, FPS, TOTAL_FRAMES } from "./Demo";
import { Demo3, TOTAL_FRAMES as TOTAL3 } from "./Demo3";

export const RemotionRoot: React.FC = () => (
  <>
    <Composition id="GloamlogDemo" component={Demo} durationInFrames={TOTAL_FRAMES} fps={FPS} width={1920} height={1080} />
    <Composition id="GloamlogDemo3" component={Demo3} durationInFrames={TOTAL3} fps={FPS} width={1920} height={1080} />
  </>
);
