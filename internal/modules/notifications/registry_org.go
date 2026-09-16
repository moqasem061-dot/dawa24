package notifications

func init() {
	// Profile & Organization Changes
	registerEvent(EventDefinition{
		Key:                EventOrgProfileChangeRequested,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "admin.organizations.manage",
		TitleAr:            "طلب تعديل بيانات المنشأة {org_name}",
		TitleEn:            "Profile Change Requested: {org_name}",
		BodyAr:             "قدمت المنشأة {org_name} طلباً لتعديل بيانات ملفها ({section}) بانتظار مراجعة الإدارة.",
		BodyEn:             "Organization {org_name} requested profile changes ({section}) awaiting admin review.",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgProfileChangeApproved,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تمت الموافقة على تعديل بيانات المنشأة",
		TitleEn:            "Profile Change Approved",
		BodyAr:             "وافقت إدارة المنصة على طلب تعديل بيانات المنشأة في قسم ({section}).",
		BodyEn:             "Platform admins approved your profile change request for section ({section}).",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgProfileChangeRejected,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تم رفض تعديل بيانات المنشأة",
		TitleEn:            "Profile Change Rejected",
		BodyAr:             "تم رفض طلب تعديل بيانات المنشأة لقسم ({section}). السبب: {reason}",
		BodyEn:             "Profile change request for section ({section}) was rejected. Reason: {reason}",
	})

	// Organization Deletion
	registerEvent(EventDefinition{
		Key:                EventOrgDeletionRequested,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "admin.organizations.manage",
		TitleAr:            "طلب حذف منشأة: {org_name}",
		TitleEn:            "Organization Deletion Requested: {org_name}",
		BodyAr:             "قدم مالك المنشأة {org_name} طلباً لحذف حساب المنشأة. السبب: {reason}",
		BodyEn:             "The owner of organization {org_name} requested organization deletion. Reason: {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgDeletionApproved,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "تمت الموافقة على حذف المنشأة",
		TitleEn:            "Organization Deletion Approved",
		BodyAr:             "تمت الموافقة على حذف المنشأة {org_name} من قبل إدارة المنصة وجارٍ تنفيذ الإجراءات.",
		BodyEn:             "Organization {org_name} deletion has been approved by platform administration.",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgDeletionRejected,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تم رفض طلب حذف المنشأة",
		TitleEn:            "Organization Deletion Rejected",
		BodyAr:             "تم رفض طلب حذف المنشأة {org_name}. السبب: {reason}",
		BodyEn:             "Organization {org_name} deletion request was rejected. Reason: {reason}",
	})

	// Account Deletion
	registerEvent(EventDefinition{
		Key:                EventAccountDeletionRequested,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "admin.users.manage",
		TitleAr:            "طلب حذف حساب مستخدم: {user_name}",
		TitleEn:            "User Account Deletion Requested: {user_name}",
		BodyAr:             "قدم المستخدم {user_name} ({user_email}) طلباً لحذف حسابه. السبب: {reason}",
		BodyEn:             "User {user_name} ({user_email}) requested account deletion. Reason: {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventAccountDeletionApproved,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "تمت الموافقة على حذف حسابك",
		TitleEn:            "Account Deletion Approved",
		BodyAr:             "تمت الموافقة على طلب حذف حسابك من قبل إدارة المنصة وسيتم إنهاء جلساتك.",
		BodyEn:             "Your account deletion request has been approved by platform administration.",
	})
	registerEvent(EventDefinition{
		Key:                EventAccountDeletionRejected,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "تم رفض طلب حذف حسابك",
		TitleEn:            "Account Deletion Rejected",
		BodyAr:             "تم رفض طلب حذف حسابك الشخصي. السبب: {reason}",
		BodyEn:             "Your account deletion request was rejected. Reason: {reason}",
	})

	// Organization Lifecycle
	registerEvent(EventDefinition{
		Key:                EventOrgApproved,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تهانينا! تم اعتماد منشأتك",
		TitleEn:            "Congratulations! Organization Approved",
		BodyAr:             "تم اعتماد المنشأة رسمياً وتفعيل وصولك الكامل لكافة مميزات منصة دواء24.",
		BodyEn:             "Your organization has been officially approved with full access to Dawa24 platform.",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgRejected,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تم رفض طلب تسجيل المنشأة",
		TitleEn:            "Organization Registration Rejected",
		BodyAr:             "تم رفض طلب تسجيل المنشأة من قبل إدارة المنصة. {reason}",
		BodyEn:             "Organization registration was rejected by platform administration. {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgSuspended,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تم إيقاف المنشأة مؤقتاً",
		TitleEn:            "Organization Suspended",
		BodyAr:             "تم إيقاف حساب منشأتك مؤقتاً من قبل إدارة المنصة. {reason}",
		BodyEn:             "Your organization account has been suspended by platform administration. {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgReactivated,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تمت إعادة تفعيل المنشأة",
		TitleEn:            "Organization Reactivated",
		BodyAr:             "تمت إعادة تفعيل حساب المنشأة بنجاح ويمكنك استئناف الأنشطة والمعاملات.",
		BodyEn:             "Your organization account has been reactivated. You may resume normal activity.",
	})

	// Documents
	registerEvent(EventDefinition{
		Key:                EventDocumentRequested,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.document.view",
		TitleAr:            "طلب مستند رسمي من الإدارة",
		TitleEn:            "Document Requested by Admin",
		BodyAr:             "طالبت إدارة المنصة تقديم مستند ({document_name}): {description}. الموعد النهائي: {deadline_days} يوماً.",
		BodyEn:             "Platform admins requested document ({document_name}): {description}. Deadline: {deadline_days} days.",
	})
	registerEvent(EventDefinition{
		Key:                EventDocumentVerified,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.document.view",
		TitleAr:            "تم توثيق واعتماد المستند",
		TitleEn:            "Document Verified",
		BodyAr:             "تم توثيق المستند ({document_name}) واعتماده بنجاح.",
		BodyEn:             "Document ({document_name}) has been successfully verified.",
	})
	registerEvent(EventDefinition{
		Key:                EventDocumentRejected,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.document.view",
		TitleAr:            "تم رفض المستند المقدم",
		TitleEn:            "Document Rejected",
		BodyAr:             "تم رفض المستند ({document_name}). السبب: {reason}",
		BodyEn:             "Document ({document_name}) was rejected. Reason: {reason}",
	})

	// Branches & Institutional Works
	registerEvent(EventDefinition{
		Key:                EventBranchCreated,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "إضافة فرع جديد: {branch_name}",
		TitleEn:            "New Branch Added: {branch_name}",
		BodyAr:             "تمت إضافة الفرع الجديد '{branch_name}' (كود: {branch_code}) بنجاح إلى منظومة منشأتك.",
		BodyEn:             "New branch '{branch_name}' (code: {branch_code}) was added to your organization.",
	})
	registerEvent(EventDefinition{
		Key:                EventBranchDisabled,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تعطيل فرع: {branch_name}",
		TitleEn:            "Branch Disabled: {branch_name}",
		BodyAr:             "تم تعطيل الفرع '{branch_name}' مؤقتاً وإيقاف استقبال الطلبات عليه.",
		BodyEn:             "Branch '{branch_name}' has been temporarily disabled.",
	})
	registerEvent(EventDefinition{
		Key:                EventBranchInstitutionalWorksChanged,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تحديث الأعمال المؤسسية للفرع {branch_name}",
		TitleEn:            "Institutional Works Updated for {branch_name}",
		BodyAr:             "تم تحديث إعدادات الأعمال المؤسسية (التأمين والخدمات) للفرع '{branch_name}'.",
		BodyEn:             "Institutional works settings (insurance and coverage) updated for branch '{branch_name}'.",
	})

	// Team & Identity
	registerEvent(EventDefinition{
		Key:                EventUserAddedToOrg,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "إضافة عضو جديد للمنشأة",
		TitleEn:            "New Member Added to Organization",
		BodyAr:             "تمت إضافة {user_name} إلى منشأة {org_name} بدور: {role_name}.",
		BodyEn:             "User {user_name} was added to {org_name} with role: {role_name}.",
	})
	registerEvent(EventDefinition{
		Key:                EventUserRemovedFromOrg,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "إزالة عضو من المنشأة",
		TitleEn:            "Member Removed from Organization",
		BodyAr:             "تمت إزالة العضو {user_name} من منشأة {org_name}.",
		BodyEn:             "Member {user_name} was removed from organization {org_name}.",
	})
	registerEvent(EventDefinition{
		Key:                EventUserRoleChanged,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "تحديث الصلاحيات والدور الوظيفي",
		TitleEn:            "Role & Permissions Updated",
		BodyAr:             "تم تحديث دورك الوظيفي في المنشأة إلى: {new_role}.",
		BodyEn:             "Your organization role has been updated to: {new_role}.",
	})
	registerEvent(EventDefinition{
		Key:                EventUserPasswordSetByAdmin,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "تعيين كلمة مرور جديدة لحسابك",
		TitleEn:            "Password Set by Administrator",
		BodyAr:             "قام مسؤول المنصة بتعيين كلمة مرور جديدة لحسابك وتم إنهاء جميع الجلسات النشطة.",
		BodyEn:             "A platform administrator set a new password for your account and terminated all active sessions.",
	})
	registerEvent(EventDefinition{
		Key:                EventAccountRegistered,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "مرحباً بك في منصة دواء24",
		TitleEn:            "Welcome to Dawa24",
		BodyAr:             "تم إنشاء حسابك بنجاح. نحن سعداء بانضمامك لمنظومة دواء24 الرقمية للرعاية الصيدلانية.",
		BodyEn:             "Your account was created successfully. Welcome to Dawa24 healthcare network.",
	})
	registerEvent(EventDefinition{
		Key:                EventAdminsNewRegistration,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "admin.organizations.manage",
		TitleAr:            "تسجيل منشأة جديدة بانتظار المراجعة",
		TitleEn:            "New Organization Registered",
		BodyAr:             "تم تسجيل حساب منشأة جديد: {org_name} ({account_type}) ويتطلب التحقق والمراجعة.",
		BodyEn:             "A new organization registered: {org_name} ({account_type}) awaiting verification.",
	})
	registerEvent(EventDefinition{
		Key:                EventAdminsNewJobSeekerRegistration,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "admin.organizations.manage",
		TitleAr:            "تسجيل باحث عن عمل جديد بانتظار المراجعة",
		TitleEn:            "New Job Seeker Registered",
		BodyAr:             "تم تسجيل حساب باحث عن عمل جديد: {user_name} ويتطلب التحقق والمراجعة.",
		BodyEn:             "A new job seeker registered: {user_name} awaiting verification.",
	})

	// Subscriptions & Billing
	registerEvent(EventDefinition{
		Key:                EventSubscriptionUpdated,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.billing.view",
		TitleAr:            "تأكيد الاشتراك في الباقة",
		TitleEn:            "Subscription Activated",
		BodyAr:             "تم تفعيل باقة {plan_name} ({cycle}) بنجاح وخصم {cost} ج.م من المحفظة.",
		BodyEn:             "Subscription to {plan_name} ({cycle}) activated successfully with {cost} EGP deducted.",
	})
	registerEvent(EventDefinition{
		Key:                EventSubscriptionExpiring,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.billing.view",
		TitleAr:            "تنبيه: اقتراب انتهاء باقة الاشتراك",
		TitleEn:            "Subscription Expiring Soon",
		BodyAr:             "سينتهي اشتراكك في باقة {plan_name} خلال {days_left} أيام. يرجى تجديد الاشتراك لتجنب توقف الميزات.",
		BodyEn:             "Your subscription for {plan_name} expires in {days_left} days. Please renew to keep full access.",
	})
	registerEvent(EventDefinition{
		Key:                EventSubscriptionExpired,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.billing.view",
		TitleAr:            "انتهت صلاحية باقة الاشتراك",
		TitleEn:            "Subscription Expired",
		BodyAr:             "انتهت صلاحية باقة {plan_name}. يرجى إعادة الاشتراك للاستمرار في الاستفادة من كافة الخدمات.",
		BodyEn:             "Your subscription for {plan_name} has expired. Please renew to resume services.",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgExtraDevicesGranted,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "تمت إضافة جلسات متزامنة إضافية لمنشأتك",
		TitleEn:            "Extra Concurrent Sessions Added",
		BodyAr:             "قام مسؤول المنصة بمنح منشأتك {extra_devices} جلسة متزامنة إضافية (إجمالي الجلسات المتاحة: {total_sessions}){expires_text}.",
		BodyEn:             "Platform administration added {extra_devices} extra concurrent session(s) to your organization (total sessions: {total_sessions}){expires_text}.",
	})
	registerEvent(EventDefinition{
		Key:                EventOrgExtraDevicesRevoked,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "تحديث الجلسات المتزامنة لمنشأتك",
		TitleEn:            "Extra Concurrent Sessions Updated",
		BodyAr:             "تم تحديث الجلسات المتزامنة لمنشأتك من قبل إدارة المنصة (إجمالي الجلسات المتاحة: {total_sessions}).",
		BodyEn:             "Concurrent sessions have been updated for your organization by platform administration (available sessions: {total_sessions}).",
	})
	registerEvent(EventDefinition{
		Key:                EventRefundIssued,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.billing.view",
		TitleAr:            "تم إصدار استرداد مالي (Refund)",
		TitleEn:            "Refund Issued",
		BodyAr:             "تم إصدار واسترداد مبلغ {amount} ج.م إلى محفظتك للطلب #{order_number}. السبب: {reason}",
		BodyEn:             "A refund of {amount} EGP was credited to your wallet for order #{order_number}. Reason: {reason}",
	})

	// Wallet
	registerEvent(EventDefinition{
		Key:                EventWalletDepositPending,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.wallet.view",
		TitleAr:            "طلب إيداع رصيد قيد المراجعة",
		TitleEn:            "Deposit Request Pending",
		BodyAr:             "تم استلام طلب شحن الرصيد بمبلغ {amount} ج.م وجارٍ مراجعته من قبل إدارة المالية.",
		BodyEn:             "Deposit request of {amount} EGP received and is under financial review.",
	})
	registerEvent(EventDefinition{
		Key:                EventWalletDepositApproved,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.wallet.view",
		TitleAr:            "تم إيداع الرصيد بنجاح",
		TitleEn:            "Wallet Deposit Approved",
		BodyAr:             "تمت إضافة مبلغ {amount} ج.م إلى محفظتك بنجاح.",
		BodyEn:             "An amount of {amount} EGP has been credited to your wallet.",
	})
	registerEvent(EventDefinition{
		Key:                EventWalletDepositRejected,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.wallet.view",
		TitleAr:            "تم رفض طلب إيداع الرصيد",
		TitleEn:            "Wallet Deposit Rejected",
		BodyAr:             "تم رفض طلب إيداع الرصيد بمبلغ {amount} ج.م. {reason}",
		BodyEn:             "Wallet deposit of {amount} EGP was rejected. {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventWalletWithdrawalPending,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.wallet.view",
		TitleAr:            "طلب سحب رصيد قيد المعالجة",
		TitleEn:            "Withdrawal Request Pending",
		BodyAr:             "تم استلام طلب سحب مبلغ {amount} ج.م وهو قيد المراجعة والتحويل البنكي.",
		BodyEn:             "Withdrawal request of {amount} EGP received and is pending bank transfer.",
	})
	registerEvent(EventDefinition{
		Key:                EventWalletWithdrawalApproved,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.wallet.view",
		TitleAr:            "تمت الموافقة على سحب الرصيد",
		TitleEn:            "Withdrawal Approved",
		BodyAr:             "تم تحويل مبلغ {amount} ج.م إلى حسابك البنكي بنجاح.",
		BodyEn:             "An amount of {amount} EGP has been successfully transferred to your bank account.",
	})
	registerEvent(EventDefinition{
		Key:                EventWalletWithdrawalRejected,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.wallet.view",
		TitleAr:            "تم رفض طلب سحب الرصيد",
		TitleEn:            "Withdrawal Rejected",
		BodyAr:             "تم رفض طلب سحب الرصيد بمبلغ {amount} ج.م. {reason}",
		BodyEn:             "Withdrawal request of {amount} EGP was rejected. {reason}",
	})

	// Reviews, Chat, HR
	registerEvent(EventDefinition{
		Key:                EventOrgReviewCreated,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.organization.view",
		TitleAr:            "تقييم ورأي جديد لمنشأتك",
		TitleEn:            "New Review for Your Organization",
		BodyAr:             "حصلت منشأتك على تقييم جديد ({rating}/5 نجوم): '{review_text}'.",
		BodyEn:             "Your organization received a new review ({rating}/5 stars): '{review_text}'.",
	})
	registerEvent(EventDefinition{
		Key:                EventChatMessageOffline,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "",
		TitleAr:            "رسالة محادثة جديدة أثناء غيابك",
		TitleEn:            "New Offline Chat Message",
		BodyAr:             "أرسل إليك {sender_name} رسالة جديدة: '{message_snippet}'.",
		BodyEn:             "User {sender_name} sent you a message: '{message_snippet}'.",
	})
	registerEvent(EventDefinition{
		Key:                EventJobApplicationReceived,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "hr.job.manage",
		TitleAr:            "طلب توظيف جديد لوظيفة: {job_title}",
		TitleEn:            "New Job Application: {job_title}",
		BodyAr:             "قدم المتقدم {applicant_name} ({applicant_phone}) طلباً لشغل وظيفة '{job_title}'.",
		BodyEn:             "Applicant {applicant_name} ({applicant_phone}) applied for job '{job_title}'.",
	})
}
