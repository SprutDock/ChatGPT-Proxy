#import "SetupProgressWindow.h"

@interface SetupProgressWindow ()
@property (nonatomic, strong) NSWindow *window;
@property (nonatomic, strong) NSTextField *statusLabel;
@property (nonatomic, strong) NSProgressIndicator *progressIndicator;
@property (nonatomic, strong) NSSecureTextField *invitationField;
@property (nonatomic, strong) NSButton *primaryButton;
@property (nonatomic, strong) NSButton *cancelButton;
@property (nonatomic) BOOL awaitingInvitation;
@end

@implementation SetupProgressWindow

- (BOOL)isVisible {
    return self.window != nil;
}

- (void)createWindow {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    self.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 480, 190)
        styleMask:NSWindowStyleMaskTitled backing:NSBackingStoreBuffered defer:NO];
    self.window.title = @"ChatGPT Proxy";
    self.window.releasedWhenClosed = NO;
    [self.window standardWindowButton:NSWindowCloseButton].hidden = YES;

    NSView *content = self.window.contentView;
    self.statusLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(20, 145, 440, 24)];
    self.statusLabel.editable = NO;
    self.statusLabel.bezeled = NO;
    self.statusLabel.drawsBackground = NO;
    self.statusLabel.font = [NSFont systemFontOfSize:14];
    [content addSubview:self.statusLabel];

    self.progressIndicator = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(20, 112, 440, 20)];
    self.progressIndicator.style = NSProgressIndicatorStyleBar;
    self.progressIndicator.minValue = 0;
    self.progressIndicator.maxValue = 100;
    self.progressIndicator.indeterminate = YES;
    [self.progressIndicator startAnimation:nil];
    [content addSubview:self.progressIndicator];

    self.invitationField = [[NSSecureTextField alloc] initWithFrame:NSMakeRect(20, 72, 440, 25)];
    self.invitationField.placeholderString = @"Приглашение";
    self.invitationField.hidden = YES;
    [content addSubview:self.invitationField];

    self.cancelButton = [[NSButton alloc] initWithFrame:NSMakeRect(235, 20, 105, 32)];
    self.cancelButton.title = @"Отмена";
    self.cancelButton.bezelStyle = NSBezelStyleRounded;
    self.cancelButton.target = self;
    self.cancelButton.action = @selector(cancelAction:);
    self.cancelButton.hidden = YES;
    [content addSubview:self.cancelButton];

    self.primaryButton = [[NSButton alloc] initWithFrame:NSMakeRect(350, 20, 110, 32)];
    self.primaryButton.title = @"Продолжить";
    self.primaryButton.bezelStyle = NSBezelStyleRounded;
    self.primaryButton.target = self;
    self.primaryButton.action = @selector(primaryAction:);
    self.primaryButton.hidden = YES;
    [content addSubview:self.primaryButton];

    [self.window center];
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (void)setIndeterminateStatus:(NSString *)status {
    self.statusLabel.stringValue = status;
    self.progressIndicator.indeterminate = YES;
    [self.progressIndicator startAnimation:nil];
}

- (void)setCompletedStage:(NSString *)status progress:(double)value {
    self.statusLabel.stringValue = status;
    [self.progressIndicator stopAnimation:nil];
    self.progressIndicator.indeterminate = NO;
    self.progressIndicator.doubleValue = value;
}

- (void)showChecking {
    if (self.window != nil) {
        self.progressIndicator.hidden = NO;
        self.invitationField.hidden = YES;
        self.cancelButton.hidden = YES;
        self.primaryButton.hidden = YES;
        self.cancelButton.title = @"Отмена";
        [self setIndeterminateStatus:@"Проверка сертификата…"];
    }
}

- (void)handleEvent:(NSDictionary *)event {
    if ([event[@"v"] integerValue] != 1 || ![event[@"event"] isKindOfClass:NSString.class]) {
        return;
    }
    NSString *name = event[@"event"];
    if ([name isEqualToString:@"checking"]) {
        if (self.window == nil) {
            [self createWindow];
        }
        [self setIndeterminateStatus:@"Проверка сертификата…"];
    } else if ([name isEqualToString:@"setup_started"]) {
        if (self.window == nil) {
            [self createWindow];
        }
        [self setIndeterminateStatus:@"Настройка защищённого прокси…"];
    } else if ([name isEqualToString:@"request_prepared"]) {
        [self setCompletedStage:@"Запрос регистрации подготовлен" progress:25];
    } else if ([name isEqualToString:@"enrolling"]) {
        [self setIndeterminateStatus:@"Регистрация устройства…"];
    } else if ([name isEqualToString:@"renewing"]) {
        [self setIndeterminateStatus:@"Обновление сертификата…"];
    } else if ([name isEqualToString:@"certificate_saved"]) {
        [self setCompletedStage:@"Сертификат сохранён" progress:50];
    } else if ([name isEqualToString:@"tunnel_starting"]) {
        [self setIndeterminateStatus:@"Запуск защищённого туннеля…"];
    } else if ([name isEqualToString:@"verifying"]) {
        [self setIndeterminateStatus:@"Проверка защищённого соединения…"];
    } else if ([name isEqualToString:@"invitation_required"]) {
        if (self.window == nil) {
            [self createWindow];
        }
        self.awaitingInvitation = YES;
        self.invitationField.hidden = NO;
        self.cancelButton.hidden = NO;
        self.primaryButton.hidden = NO;
        self.primaryButton.title = @"Продолжить";
        self.statusLabel.stringValue = @"Введите приглашение для регистрации устройства.";
        [self.window makeFirstResponder:self.invitationField];
    } else if ([name isEqualToString:@"ready"] && self.window != nil) {
        self.awaitingInvitation = NO;
        self.cancelButton.hidden = YES;
        self.primaryButton.hidden = YES;
        self.invitationField.hidden = YES;
        [self setCompletedStage:@"Настройка завершена" progress:100];
    }
}

- (void)showFailure:(NSString *)reason {
    if (self.window == nil) {
        [self createWindow];
    }
    self.statusLabel.stringValue = reason;
    self.progressIndicator.hidden = YES;
    self.invitationField.hidden = YES;
    self.awaitingInvitation = NO;
    self.cancelButton.title = @"Закрыть";
    self.cancelButton.hidden = NO;
    self.primaryButton.title = @"Повторить";
    self.primaryButton.hidden = NO;
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (void)showFinished {
    self.awaitingInvitation = NO;
    self.cancelButton.hidden = YES;
    self.primaryButton.hidden = YES;
    self.invitationField.hidden = YES;
    self.progressIndicator.hidden = NO;
    [self setCompletedStage:@"Прокси готов. Запускаем ChatGPT…" progress:100];
}

- (void)primaryAction:(id)sender {
    if (!self.awaitingInvitation) {
        if (self.retryHandler != nil) {
            self.retryHandler();
        }
        return;
    }
    NSString *invitation = self.invitationField.stringValue;
    NSUInteger length = [invitation lengthOfBytesUsingEncoding:NSUTF8StringEncoding];
    if (length < 32 || length > 256) {
        self.statusLabel.stringValue = @"Проверьте длину приглашения и повторите ввод.";
        return;
    }
    self.invitationField.stringValue = @"";
    self.invitationField.hidden = YES;
    self.cancelButton.hidden = YES;
    self.primaryButton.hidden = YES;
    self.awaitingInvitation = NO;
    if (self.invitationHandler != nil) {
        self.invitationHandler(invitation);
    }
    [self setIndeterminateStatus:@"Подготовка запроса регистрации…"];
}

- (void)cancelAction:(id)sender {
    if (self.cancelHandler != nil) {
        self.cancelHandler();
    }
}

@end
