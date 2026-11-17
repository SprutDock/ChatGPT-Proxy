#import <AppKit/AppKit.h>
#import "ChatGPTProxyUpdater.h"

@interface AppDelegate : NSObject <NSApplicationDelegate>
@property (nonatomic, strong) ChatGPTProxyUpdater *updater;
@end

@implementation AppDelegate

- (void)showError:(NSString *)message {
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"ChatGPT Proxy не запущен";
    alert.informativeText = message.length > 0 ? message : @"Проверьте ~/.codex/.env и proxy-url.txt.";
    [alert addButtonWithTitle:@"OK"];
    [NSApp activateIgnoringOtherApps:YES];
    [alert runModal];
    [NSApp terminate:nil];
}

- (void)launchProxy {
    NSString *scriptPath = [[NSBundle mainBundle] pathForResource:@"launch" ofType:@"sh"];
    if (scriptPath == nil) {
        [self showError:@"В приложении отсутствует launch.sh."];
        return;
    }

    NSTask *task = [[NSTask alloc] init];
    NSPipe *output = [NSPipe pipe];
    task.executableURL = [NSURL fileURLWithPath:@"/bin/zsh"];
    task.arguments = @[scriptPath];
    task.standardOutput = output;
    task.standardError = output;
    task.terminationHandler = ^(NSTask *finishedTask) {
        NSData *data = [output.fileHandleForReading readDataToEndOfFile];
        NSString *message = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        dispatch_async(dispatch_get_main_queue(), ^{
            if (finishedTask.terminationStatus == 0) {
                [NSApp terminate:nil];
            } else {
                [self showError:message];
            }
        });
    };

    NSError *error = nil;
    if (![task launchAndReturnError:&error]) {
        [self showError:error.localizedDescription];
    }
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    self.updater = [[ChatGPTProxyUpdater alloc] init];
    __weak typeof(self) weakSelf = self;
    [self.updater checkForUpdatesWithCompletion:^{
        [weakSelf launchProxy];
    }];
}

@end

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        NSApplication *application = [NSApplication sharedApplication];
        AppDelegate *delegate = [[AppDelegate alloc] init];
        application.delegate = delegate;
        [application setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [application run];
    }
    return 0;
}
