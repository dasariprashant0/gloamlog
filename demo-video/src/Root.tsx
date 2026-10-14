import React from "react";
import { Composition } from "remotion";
import { Demo, FPS, TOTAL_FRAMES, Title } from "./Demo";

export const RemotionRoot: React.FC = () => (
  <>
    <Composition id="GloamlogDemo" component={Demo} durationInFrames={TOTAL_FRAMES} fps={FPS} width={1920} height={1080} />
    <Composition id="TitleOnly" component={Title} durationInFrames={150} fps={FPS} width={1920} height={1080} />
  </>
);
