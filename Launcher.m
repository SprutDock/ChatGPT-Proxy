#import <AppKit/AppKit.h>

static NSString *const GitHubLatestReleaseAPI = @"https://api.github.com/repos/SprutDock/ChatGPT-Proxy/releases/latest";
static NSString *const GitHubLatestReleasePage = @"https://github.com/SprutDock/ChatGPT-Proxy/releases/latest";

@interface AppDelegate : NSObject <NSApplicationDelegate>
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

- (void)checkForUpdatesThenLaunchProxy {
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:[NSURL URLWithString:GitHubLatestReleaseAPI]];
    request.timeoutInterval = 4.0;
    request.cachePolicy = NSURLRequestReloadIgnoringLocalCacheData;
    [request setValue:@"application/vnd.github+json" forHTTPHeaderField:@"Accept"];
    [request setValue:@"ChatGPT-Proxy" forHTTPHeaderField:@"User-Agent"];

    NSURLSessionDataTask *updateCheck = [[NSURLSession sharedSession] dataTaskWithRequest:request completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
        NSHTTPURLResponse *httpResponse = (NSHTTPURLResponse *)response;
        NSDictionary *release = data == nil ? nil : [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        id tag = [release isKindOfClass:[NSDictionary class]] ? release[@"tag_name"] : nil;
        NSString *currentVersion = [[NSBundle mainBundle] objectForInfoDictionaryKey:@"CFBundleShortVersionString"];
        NSString *latestVersion = [tag isKindOfClass:[NSString class]] && [tag hasPrefix:@"v"] ? [tag substringFromIndex:1] : tag;

        if (error != nil || httpResponse.statusCode != 200 || ![latestVersion isKindOfClass:[NSString class]] || ![currentVersion isKindOfClass:[NSString class]] || [latestVersion compare:currentVersion options:NSNumericSearch] != NSOrderedDescending) {
            dispatch_async(dispatch_get_main_queue(), ^{
                [self launchProxy];
            });
            return;
        }

        dispatch_async(dispatch_get_main_queue(), ^{
            NSAlert *alert = [[NSAlert alloc] init];
            alert.messageText = @"Доступна новая версия ChatGPT Proxy";
            alert.informativeText = [NSString stringWithFormat:@"Установлена версия %@. На GitHub доступна версия %@.", currentVersion, latestVersion];
            [alert addButtonWithTitle:@"Открыть релиз"];
            [alert addButtonWithTitle:@"Позже"];
            [NSApp activateIgnoringOtherApps:YES];
            if ([alert runModal] == NSAlertFirstButtonReturn) {
                [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:GitHubLatestReleasePage]];
            }
            [self launchProxy];
        });
    }];
    [updateCheck resume];
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    [self checkForUpdatesThenLaunchProxy];
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
