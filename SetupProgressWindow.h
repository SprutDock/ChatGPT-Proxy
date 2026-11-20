#import <AppKit/AppKit.h>

@interface SetupProgressWindow : NSObject
@property (nonatomic, copy) void (^invitationHandler)(NSString *invitation);
@property (nonatomic, copy) void (^retryHandler)(void);
@property (nonatomic, copy) void (^cancelHandler)(void);
@property (nonatomic, readonly, getter=isVisible) BOOL visible;
- (void)handleEvent:(NSDictionary *)event;
- (void)showChecking;
- (void)showFailure:(NSString *)reason;
- (void)showFinished;
@end
