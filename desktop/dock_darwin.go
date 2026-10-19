package main

// The Dock bounce when a reminder brings Gloamlog forward (the Swift app's requestUserAttention), skipped while Reduce Motion
// is on. Wails has no API for it, so this asks AppKit directly, on the main thread.

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void gloamlogBounceDock(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceMotion) return;
		[NSApp requestUserAttention:NSInformationalRequest];
	});
}
*/
import "C"

func bounceDock() { C.gloamlogBounceDock() }
