#import "ChatGPTProxyUpdater.h"
#import <AppKit/AppKit.h>
#import <CommonCrypto/CommonDigest.h>
#import <unistd.h>

static NSString *const GitHubLatestReleaseAPI = @"https://api.github.com/repos/SprutDock/ChatGPT-Proxy/releases/latest";
static NSString *const UpdateArchiveName = @"ChatGPT-Proxy-macOS.zip";
static NSString *const AppBundleIdentifier = @"com.sprutdock.chatgpt-proxy-launcher";
static unsigned long long const MaximumUpdateSize = 100ULL * 1024 * 1024;

@interface ChatGPTProxyUpdater ()
@property (nonatomic, strong) NSWindow *updateWindow;
@property (nonatomic, copy) void (^completion)(void);
@end

@implementation ChatGPTProxyUpdater

- (void)showUpdateError:(NSString *)message {
    [self.updateWindow close];
    self.updateWindow = nil;
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"Не удалось установить обновление";
    alert.informativeText = message.length > 0 ? message : @"Текущая версия приложения не заменена.";
    [alert addButtonWithTitle:@"OK"];
    [NSApp activateIgnoringOtherApps:YES];
    [alert runModal];
}

- (void)continueAfterUpdate {
    void (^completion)(void) = self.completion;
    self.completion = nil;
    if (completion != nil) {
        completion();
    }
}

- (NSError *)updateFailure:(NSString *)message {
    return [NSError errorWithDomain:@"ChatGPTProxyUpdater" code:1 userInfo:@{NSLocalizedDescriptionKey: message}];
}

- (BOOL)runTask:(NSString *)executablePath arguments:(NSArray<NSString *> *)arguments {
    NSTask *task = [[NSTask alloc] init];
    task.executableURL = [NSURL fileURLWithPath:executablePath];
    task.arguments = arguments;
    NSError *error = nil;
    if (![task launchAndReturnError:&error]) {
        return NO;
    }
    [task waitUntilExit];
    return task.terminationStatus == 0;
}

- (NSString *)sha256ForFileAtPath:(NSString *)path {
    NSInputStream *stream = [NSInputStream inputStreamWithFileAtPath:path];
    [stream open];
    CC_SHA256_CTX context;
    CC_SHA256_Init(&context);
    uint8_t buffer[65536];
    NSInteger bytesRead = 0;
    while ((bytesRead = [stream read:buffer maxLength:sizeof(buffer)]) > 0) {
        CC_SHA256_Update(&context, buffer, (CC_LONG)bytesRead);
    }
    [stream close];
    if (bytesRead < 0) {
        return nil;
    }
    unsigned char digest[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256_Final(digest, &context);
    NSMutableString *result = [NSMutableString stringWithCapacity:CC_SHA256_DIGEST_LENGTH * 2];
    for (NSUInteger index = 0; index < CC_SHA256_DIGEST_LENGTH; index++) {
        [result appendFormat:@"%02x", digest[index]];
    }
    return result;
}

- (void)showUpdateProgressForVersion:(NSString *)version {
    NSWindow *window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 380, 105) styleMask:NSWindowStyleMaskTitled backing:NSBackingStoreBuffered defer:NO];
    window.title = @"ChatGPT Proxy";
    NSTextField *message = [NSTextField labelWithString:[NSString stringWithFormat:@"Скачиваю версию %@…", version]];
    message.frame = NSMakeRect(20, 65, 340, 20);
    NSProgressIndicator *spinner = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(20, 25, 20, 20)];
    spinner.style = NSProgressIndicatorStyleSpinning;
    spinner.indeterminate = YES;
    [spinner startAnimation:nil];
    [window.contentView addSubview:message];
    [window.contentView addSubview:spinner];
    [window center];
    [window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
    self.updateWindow = window;
}

- (void)finishUpdateWithError:(NSError *)error stagingDirectoryURL:(NSURL *)stagingDirectoryURL {
    [[NSFileManager defaultManager] removeItemAtURL:stagingDirectoryURL error:nil];
    [self showUpdateError:error.localizedDescription];
    [self continueAfterUpdate];
}

- (void)installStagedApplicationAtURL:(NSURL *)replacementURL stagingDirectoryURL:(NSURL *)stagingDirectoryURL {
    NSFileManager *fileManager = [NSFileManager defaultManager];
    NSURL *applicationURL = [[NSBundle mainBundle] bundleURL];
    NSURL *parentURL = [applicationURL URLByDeletingLastPathComponent];
    NSURL *sourceScriptURL = [[NSBundle mainBundle] URLForResource:@"update" withExtension:@"sh"];
    NSURL *helperScriptURL = [stagingDirectoryURL URLByAppendingPathComponent:@"update.sh"];
    NSError *error = nil;
    if (sourceScriptURL == nil || ![fileManager copyItemAtURL:sourceScriptURL toURL:helperScriptURL error:&error]) {
        [self finishUpdateWithError:error ?: [self updateFailure:@"Не найден скрипт установки."] stagingDirectoryURL:stagingDirectoryURL];
        return;
    }
    if (![fileManager setAttributes:@{NSFilePosixPermissions: @0755} ofItemAtPath:helperScriptURL.path error:&error]) {
        [self finishUpdateWithError:error stagingDirectoryURL:stagingDirectoryURL];
        return;
    }

    NSString *backupName = [NSString stringWithFormat:@".ChatGPT-Proxy-backup-%@", [[NSUUID UUID] UUIDString]];
    NSURL *backupURL = [parentURL URLByAppendingPathComponent:backupName isDirectory:YES];
    NSTask *installer = [[NSTask alloc] init];
    installer.executableURL = [NSURL fileURLWithPath:@"/bin/zsh"];
    installer.arguments = @[helperScriptURL.path, applicationURL.path, replacementURL.path, backupURL.path, stagingDirectoryURL.path, [NSString stringWithFormat:@"%d", getpid()]];
    if (![installer launchAndReturnError:&error]) {
        [self finishUpdateWithError:error stagingDirectoryURL:stagingDirectoryURL];
        return;
    }
    [self.updateWindow close];
    self.updateWindow = nil;
    [NSApp terminate:nil];
}

- (void)downloadAndInstallAsset:(NSDictionary *)asset version:(NSString *)version {
    id rawDownloadAddress = [asset isKindOfClass:[NSDictionary class]] ? asset[@"browser_download_url"] : nil;
    NSString *downloadAddress = [rawDownloadAddress isKindOfClass:[NSString class]] ? rawDownloadAddress : nil;
    NSURLComponents *components = downloadAddress == nil ? nil : [NSURLComponents componentsWithString:downloadAddress];
    NSNumber *assetSize = [asset[@"size"] isKindOfClass:[NSNumber class]] ? asset[@"size"] : nil;
    NSString *digest = [asset[@"digest"] isKindOfClass:[NSString class]] ? asset[@"digest"] : nil;
    if (![components.scheme.lowercaseString isEqualToString:@"https"] || ![components.host.lowercaseString isEqualToString:@"github.com"] || assetSize == nil || assetSize.unsignedLongLongValue == 0 || assetSize.unsignedLongLongValue > MaximumUpdateSize || ![digest hasPrefix:@"sha256:"] || digest.length != 71) {
        [self showUpdateError:@"GitHub вернул некорректные сведения об архиве."];
        [self continueAfterUpdate];
        return;
    }

    NSString *hexCharacters = [digest substringFromIndex:7].lowercaseString;
    NSCharacterSet *invalidHexCharacters = [[NSCharacterSet characterSetWithCharactersInString:@"0123456789abcdef"] invertedSet];
    if (hexCharacters.length != 64 || [hexCharacters rangeOfCharacterFromSet:invalidHexCharacters].location != NSNotFound) {
        [self showUpdateError:@"Не удалось проверить SHA-256 архива."];
        [self continueAfterUpdate];
        return;
    }

    NSURL *applicationURL = [[NSBundle mainBundle] bundleURL];
    NSURL *parentURL = [applicationURL URLByDeletingLastPathComponent];
    NSFileManager *fileManager = [NSFileManager defaultManager];
    if (![fileManager isWritableFileAtPath:parentURL.path]) {
        [self showUpdateError:@"Нет прав на замену приложения в этой папке. Переместите ChatGPT Proxy в папку ~/Applications и повторите обновление."];
        [self continueAfterUpdate];
        return;
    }

    NSString *stagingName = [NSString stringWithFormat:@".ChatGPT-Proxy-update-%@", [[NSUUID UUID] UUIDString]];
    NSURL *stagingDirectoryURL = [parentURL URLByAppendingPathComponent:stagingName isDirectory:YES];
    NSError *error = nil;
    if (![fileManager createDirectoryAtURL:stagingDirectoryURL withIntermediateDirectories:NO attributes:@{NSFilePosixPermissions: @0700} error:&error]) {
        [self showUpdateError:error.localizedDescription];
        [self continueAfterUpdate];
        return;
    }

    [self showUpdateProgressForVersion:version];
    NSURL *downloadURL = components.URL;
    NSURLSessionDownloadTask *downloadTask = [[NSURLSession sharedSession] downloadTaskWithURL:downloadURL completionHandler:^(NSURL *location, NSURLResponse *response, NSError *downloadError) {
        NSError *failure = downloadError;
        if (failure == nil && (![response isKindOfClass:[NSHTTPURLResponse class]] || ((NSHTTPURLResponse *)response).statusCode != 200)) {
            failure = [self updateFailure:@"GitHub не смог отдать архив приложения."];
        }

        NSURL *archiveURL = [stagingDirectoryURL URLByAppendingPathComponent:UpdateArchiveName];
        NSError *fileError = nil;
        if (failure == nil && (location == nil || ![fileManager copyItemAtURL:location toURL:archiveURL error:&fileError])) {
            failure = fileError ?: [self updateFailure:@"Не удалось сохранить загруженный архив."];
        }
        NSDictionary *archiveAttributes = failure == nil ? [fileManager attributesOfItemAtPath:archiveURL.path error:&fileError] : nil;
        NSNumber *downloadedSize = [archiveAttributes[NSFileSize] isKindOfClass:[NSNumber class]] ? archiveAttributes[NSFileSize] : nil;
        if (failure == nil && (downloadedSize == nil || downloadedSize.unsignedLongLongValue == 0 || downloadedSize.unsignedLongLongValue > MaximumUpdateSize)) {
            failure = [self updateFailure:@"Размер загруженного архива отличается от ожидаемого."];
        }
        NSString *actualDigest = failure == nil ? [self sha256ForFileAtPath:archiveURL.path] : nil;
        if (failure == nil && ![actualDigest isEqualToString:hexCharacters]) {
            failure = [self updateFailure:@"SHA-256 архива не совпал с данными GitHub; текущая версия оставлена без изменений."];
        }
        if (failure == nil && ![self runTask:@"/usr/bin/ditto" arguments:@[@"-x", @"-k", archiveURL.path, stagingDirectoryURL.path]]) {
            failure = [self updateFailure:@"Не удалось распаковать архив обновления."];
        }

        NSURL *replacementURL = [stagingDirectoryURL URLByAppendingPathComponent:@"ChatGPT Proxy.app" isDirectory:YES];
        NSURL *infoURL = [replacementURL URLByAppendingPathComponent:@"Contents/Info.plist"];
        NSDictionary *newInfo = [NSDictionary dictionaryWithContentsOfURL:infoURL];
        NSString *bundleIdentifier = [newInfo[@"CFBundleIdentifier"] isKindOfClass:[NSString class]] ? newInfo[@"CFBundleIdentifier"] : nil;
        NSString *newVersion = [newInfo[@"CFBundleShortVersionString"] isKindOfClass:[NSString class]] ? newInfo[@"CFBundleShortVersionString"] : nil;
        if (failure == nil && (![bundleIdentifier isEqualToString:AppBundleIdentifier] || ![newVersion isEqualToString:version])) {
            failure = [self updateFailure:@"Загружен пакет другого приложения или версии; текущая версия оставлена без изменений."];
        }
        if (failure == nil && ![self runTask:@"/usr/bin/codesign" arguments:@[@"--verify", @"--deep", @"--strict", replacementURL.path]]) {
            failure = [self updateFailure:@"Проверка подписи приложения не прошла; текущая версия оставлена без изменений."];
        }

        dispatch_async(dispatch_get_main_queue(), ^{
            if (failure != nil) {
                [self finishUpdateWithError:failure stagingDirectoryURL:stagingDirectoryURL];
                return;
            }
            [self installStagedApplicationAtURL:replacementURL stagingDirectoryURL:stagingDirectoryURL];
        });
    }];
    [downloadTask resume];
}

- (void)checkForUpdatesWithCompletion:(void (^)(void))completion {
    self.completion = completion;
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:[NSURL URLWithString:GitHubLatestReleaseAPI]];
    request.timeoutInterval = 4.0;
    request.cachePolicy = NSURLRequestReloadIgnoringLocalCacheData;
    [request setValue:@"application/vnd.github+json" forHTTPHeaderField:@"Accept"];
    [request setValue:@"ChatGPT-Proxy" forHTTPHeaderField:@"User-Agent"];

    NSURLSessionDataTask *updateCheck = [[NSURLSession sharedSession] dataTaskWithRequest:request completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
        NSHTTPURLResponse *httpResponse = (NSHTTPURLResponse *)response;
        id parsedRelease = data == nil ? nil : [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        NSDictionary *release = [parsedRelease isKindOfClass:[NSDictionary class]] ? parsedRelease : nil;
        id tag = release[@"tag_name"];
        NSString *currentVersion = [[NSBundle mainBundle] objectForInfoDictionaryKey:@"CFBundleShortVersionString"];
        NSString *latestVersion = [tag isKindOfClass:[NSString class]] && [tag hasPrefix:@"v"] ? [tag substringFromIndex:1] : tag;
        NSDictionary *asset = nil;
        for (id candidate in [release[@"assets"] isKindOfClass:[NSArray class]] ? release[@"assets"] : @[]) {
            NSString *candidateName = [candidate isKindOfClass:[NSDictionary class]] && [candidate[@"name"] isKindOfClass:[NSString class]] ? candidate[@"name"] : nil;
            if ([candidateName isEqualToString:UpdateArchiveName]) {
                asset = candidate;
                break;
            }
        }

        if (error != nil || httpResponse.statusCode != 200 || ![latestVersion isKindOfClass:[NSString class]] || ![currentVersion isKindOfClass:[NSString class]] || [latestVersion compare:currentVersion options:NSNumericSearch] != NSOrderedDescending || asset == nil) {
            dispatch_async(dispatch_get_main_queue(), ^{
                [self continueAfterUpdate];
            });
            return;
        }

        dispatch_async(dispatch_get_main_queue(), ^{
            NSAlert *alert = [[NSAlert alloc] init];
            alert.messageText = @"Доступна новая версия ChatGPT Proxy";
            alert.informativeText = [NSString stringWithFormat:@"Установлена версия %@. Доступна новая версия %@.", currentVersion, latestVersion];
            [alert addButtonWithTitle:@"Скачать"];
            [alert addButtonWithTitle:@"Позже"];
            [NSApp activateIgnoringOtherApps:YES];
            if ([alert runModal] == NSAlertFirstButtonReturn) {
                [self downloadAndInstallAsset:asset version:latestVersion];
            } else {
                [self continueAfterUpdate];
            }
        });
    }];
    [updateCheck resume];
}


@end
