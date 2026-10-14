import React from "react";
import { Composition } from "remotion";
import { Demo, FPS, TOTAL_FRAMES } from "./Demo";

export const RemotionRoot: React.FC = () => (
  <Composition id="GloamlogDemo" component={Demo} durationInFrames={TOTAL_FRAMES} fps={FPS} width={1920} height={1080} />
);
