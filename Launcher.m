#import <AppKit/AppKit.h>
#include <string.h>
#import "ChatGPTProxyUpdater.h"
#import "SetupProgressWindow.h"

@interface AppDelegate : NSObject <NSApplicationDelegate>
@property (nonatomic, strong) ChatGPTProxyUpdater *updater;
@property (nonatomic, strong) SetupProgressWindow *setupUI;
@property (nonatomic, strong) NSTask *task;
@property (nonatomic, strong) NSPipe *inputPipe;
@property (nonatomic, copy) NSString *scriptPath;
@property (nonatomic, copy) NSString *currentStage;
@property (nonatomic) BOOL cancelled;
@end

@implementation AppDelegate

- (void)showError:(NSString *)message {
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"ChatGPT Proxy не запущен";
    alert.informativeText = message.length > 0 ? message : @"Настройка прокси не завершилась. Повторите запуск.";
    [alert addButtonWithTitle:@"OK"];
    [NSApp activateIgnoringOtherApps:YES];
    [alert runModal];
    [NSApp terminate:nil];
}

- (void)writeFailureLog:(NSString *)message {
    NSString *logDirectory = [NSHomeDirectory() stringByAppendingPathComponent:@"Library/Logs/SprutDock ChatGPT Proxy"];
    [[NSFileManager defaultManager] createDirectoryAtPath:logDirectory withIntermediateDirectories:YES attributes:nil error:nil];
    NSString *logPath = [logDirectory stringByAppendingPathComponent:@"launcher.log"];
    NSMutableString *clean = [NSMutableString string];
    NSCharacterSet *controls = [NSCharacterSet controlCharacterSet];
    for (NSUInteger index = 0; index < message.length && clean.length < 500; index++) {
        unichar character = [message characterAtIndex:index];
        unichar safeCharacter = [controls characterIsMember:character] ? (unichar)' ' : character;
        [clean appendFormat:@"%C", safeCharacter];
    }
    NSString *line = [NSString stringWithFormat:@"%@ stage=%@ cause=%@\n", [NSDate date], self.currentStage ?: @"unknown", clean];
    NSFileHandle *file = [NSFileHandle fileHandleForWritingAtPath:logPath];
    if (file == nil) {
        [[NSFileManager defaultManager] createFileAtPath:logPath contents:nil attributes:nil];
        file = [NSFileHandle fileHandleForWritingAtPath:logPath];
    }
    [file seekToEndOfFile];
    [file writeData:[line dataUsingEncoding:NSUTF8StringEncoding]];
    [file closeFile];
}

- (void)showSetupFailure:(NSString *)message {
    NSString *reason = message.length > 0 ? message : @"Проверка не завершилась. Проверьте сеть и повторите попытку.";
    if (reason.length > 600) {
        reason = [reason substringFromIndex:reason.length - 600];
    }
    [self writeFailureLog:reason];
    [self.setupUI showFailure:reason];
}

- (void)handleEvent:(NSDictionary *)event {
    NSString *name = event[@"event"];
    NSSet *knownEvents = [NSSet setWithArray:@[@"checking", @"setup_started", @"invitation_required", @"request_prepared", @"enrolling", @"renewing", @"certificate_saved", @"tunnel_starting", @"verifying", @"proxy_conflict", @"ready"]];
    if ([event[@"v"] integerValue] == 1 && [name isKindOfClass:NSString.class] && [knownEvents containsObject:name]) {
        self.currentStage = name;
        [self.setupUI handleEvent:event];
    }
}

- (void)readEventsFromHandle:(NSFileHandle *)handle group:(dispatch_group_t)group {
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        NSMutableData *buffer = [NSMutableData data];
        NSData *newline = [NSData dataWithBytes:"\n" length:1];
        while (YES) {
            NSData *chunk = [handle availableData];
            if (chunk.length == 0) {
                break;
            }
            [buffer appendData:chunk];
            NSRange range;
            while ((range = [buffer rangeOfData:newline options:0 range:NSMakeRange(0, buffer.length)]).location != NSNotFound) {
                NSData *line = [buffer subdataWithRange:NSMakeRange(0, range.location)];
                [buffer replaceBytesInRange:NSMakeRange(0, range.location + 1) withBytes:NULL length:0];
                NSDictionary *event = [NSJSONSerialization JSONObjectWithData:line options:0 error:nil];
                if ([event isKindOfClass:NSDictionary.class]) {
                    dispatch_async(dispatch_get_main_queue(), ^{ [self handleEvent:event]; });
                }
            }
        }
        dispatch_group_leave(group);
    });
}

- (void)readErrorsFromHandle:(NSFileHandle *)handle group:(dispatch_group_t)group {
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        while (YES) {
            NSData *chunk = [handle availableData];
            if (chunk.length == 0) {
                break;
            }
        }
        dispatch_group_leave(group);
    });
}

- (NSString *)safeFailureReason {
    NSDictionary *reasons = @{
        @"checking": @"Не удалось проверить состояние сертификата. Повторите запуск.",
        @"setup_started": @"Не удалось завершить настройку сертификата. Повторите запуск.",
        @"invitation_required": @"Регистрация не завершилась. Проверьте приглашение и повторите запуск.",
        @"request_prepared": @"Не удалось отправить запрос регистрации. Проверьте подключение к сети.",
        @"enrolling": @"Сервис регистрации недоступен или отклонил запрос. Проверьте подключение и повторите запуск.",
        @"renewing": @"Не удалось обновить сертификат. Проверьте подключение к сети.",
        @"certificate_saved": @"Не удалось активировать сертификат. Повторите запуск.",
        @"tunnel_starting": @"Не удалось запустить защищённый туннель. Повторите запуск.",
        @"verifying": @"Защищённое соединение не прошло проверку. Проверьте сеть и повторите запуск.",
        @"proxy_conflict": @"Локальный порт 3128 занят другим процессом. Закройте это приложение и повторите запуск.",
        @"ready": @"Не удалось открыть ChatGPT. Проверьте, что приложение установлено в /Applications/ChatGPT.app."
    };
    return reasons[self.currentStage] ?: reasons[@"checking"];
}

- (void)sendInvitation:(NSString *)invitation {
    NSData *data = [[invitation stringByAppendingString:@"\n"] dataUsingEncoding:NSUTF8StringEncoding];
    [self.inputPipe.fileHandleForWriting writeData:data];
    [self.inputPipe.fileHandleForWriting closeFile];
}

- (void)cancelCurrentOperation {
    self.cancelled = YES;
    if (self.task.isRunning) {
        [self.task terminate];
    } else {
        [NSApp terminate:nil];
    }
}

- (void)startLaunchProcess {
    self.cancelled = NO;
    self.currentStage = @"checking";
    [self.setupUI showChecking];

    self.task = [[NSTask alloc] init];
    self.task.executableURL = [NSURL fileURLWithPath:@"/bin/zsh"];
    self.task.arguments = @[self.scriptPath];
    NSPipe *output = [NSPipe pipe];
    NSPipe *errors = [NSPipe pipe];
    self.inputPipe = [NSPipe pipe];
    self.task.standardOutput = output;
    self.task.standardError = errors;
    self.task.standardInput = self.inputPipe;

    dispatch_group_t readers = dispatch_group_create();
    dispatch_group_enter(readers);
    dispatch_group_enter(readers);
    __weak typeof(self) weakSelf = self;
    self.task.terminationHandler = ^(NSTask *finishedTask) {
        dispatch_group_notify(readers, dispatch_get_main_queue(), ^{
            AppDelegate *self = weakSelf;
            if (self == nil) {
                return;
            }
            if (self.cancelled) {
                [NSApp terminate:nil];
            } else if (finishedTask.terminationStatus == 0) {
                if (self.setupUI.isVisible) {
                    [self.setupUI showFinished];
                    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.5 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
                        [NSApp terminate:nil];
                    });
                } else {
                    [NSApp terminate:nil];
                }
            } else if (self.setupUI != nil) {
                [self showSetupFailure:[self safeFailureReason]];
            } else {
                NSString *reason = [self safeFailureReason];
                [self writeFailureLog:reason];
                [self showError:reason];
            }
        });
    };

    NSError *error = nil;
    if (![self.task launchAndReturnError:&error]) {
        [output.fileHandleForWriting closeFile];
        [errors.fileHandleForWriting closeFile];
        dispatch_group_leave(readers);
        dispatch_group_leave(readers);
        if (self.setupUI == nil) {
            [self showError:error.localizedDescription];
        } else {
            [self showSetupFailure:error.localizedDescription];
        }
        return;
    }

    [output.fileHandleForWriting closeFile];
    [errors.fileHandleForWriting closeFile];
    [self readEventsFromHandle:output.fileHandleForReading group:readers];
    [self readErrorsFromHandle:errors.fileHandleForReading group:readers];
}

- (void)launchProxy {
    self.scriptPath = [[NSBundle mainBundle] pathForResource:@"launch" ofType:@"sh"];
    if (self.scriptPath == nil) {
        [self showError:@"В приложении отсутствует launch.sh."];
        return;
    }
    self.setupUI = [[SetupProgressWindow alloc] init];
    __weak typeof(self) weakSelf = self;
    self.setupUI.invitationHandler = ^(NSString *invitation) { [weakSelf sendInvitation:invitation]; };
    self.setupUI.retryHandler = ^{ [weakSelf startLaunchProcess]; };
    self.setupUI.cancelHandler = ^{ [weakSelf cancelCurrentOperation]; };
    [self startLaunchProcess];
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    self.updater = [[ChatGPTProxyUpdater alloc] init];
    __weak typeof(self) weakSelf = self;
    [self.updater checkForUpdatesWithCompletion:^{
        dispatch_async(dispatch_get_main_queue(), ^{ [weakSelf launchProxy]; });
    }];
}

@end

static int openApplicationFromCommandLine(int argc, const char *argv[]) {
    if (argc < 3) {
        return 2;
    }

    NSString *applicationPath = [NSString stringWithUTF8String:argv[2]];
    if (applicationPath.length == 0) {
        return 2;
    }

    NSMutableArray<NSString *> *arguments = [NSMutableArray array];
    for (int index = 3; index < argc; index++) {
        NSString *argument = [NSString stringWithUTF8String:argv[index]];
        if (argument == nil) {
            return 2;
        }
        [arguments addObject:argument];
    }

    NSWorkspaceOpenConfiguration *configuration = [NSWorkspaceOpenConfiguration configuration];
    configuration.arguments = arguments;
    configuration.environment = [NSProcessInfo processInfo].environment;
    configuration.createsNewApplicationInstance = YES;

    dispatch_semaphore_t completion = dispatch_semaphore_create(0);
    __block int status = 1;
    [[NSWorkspace sharedWorkspace] openApplicationAtURL:[NSURL fileURLWithPath:applicationPath isDirectory:YES]
                                          configuration:configuration
                                      completionHandler:^(NSRunningApplication *application, NSError *error) {
        status = application != nil && error == nil ? 0 : 1;
        dispatch_semaphore_signal(completion);
    }];
    dispatch_semaphore_wait(completion, DISPATCH_TIME_FOREVER);
    return status;
}

int main(int argc, const char *argv[]) {
    if (argc > 1 && strcmp(argv[1], "--open-application") == 0) {
        @autoreleasepool {
            return openApplicationFromCommandLine(argc, argv);
        }
    }

    @autoreleasepool {
        NSApplication *application = [NSApplication sharedApplication];
        AppDelegate *delegate = [[AppDelegate alloc] init];
        application.delegate = delegate;
        [application setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [application run];
    }
    return 0;
}
