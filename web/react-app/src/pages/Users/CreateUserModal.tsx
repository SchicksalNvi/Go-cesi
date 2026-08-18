import { Form, Input, Modal, Switch } from 'antd';
import { Lock, Mail, User } from 'lucide-react';

interface CreateUserModalProps {
  open: boolean;
  form: any;
  t: any;
  onCancel: () => void;
  onSubmit: () => void;
}

const CreateUserModal: React.FC<CreateUserModalProps> = ({
  open,
  form,
  t,
  onCancel,
  onSubmit,
}) => {
  return (
    <Modal
      title={t.users.addUser}
      open={open}
      onOk={onSubmit}
      onCancel={onCancel}
      width={500}
    >
      <Form form={form} layout="vertical" initialValues={{ is_admin: false }}>
        <Form.Item
          name="username"
          label={t.users.username}
          rules={[
            { required: true, message: t.login.usernameRequired },
            { min: 3, max: 50, message: t.users.usernameLength },
          ]}
        >
          <Input prefix={<User size={14} strokeWidth={1.7} />} placeholder={t.users.username} />
        </Form.Item>
        <Form.Item
          name="email"
          label={t.users.email}
          rules={[
            { required: true, message: t.users.pleaseEnterEmail },
            { type: 'email', message: t.users.pleaseEnterValidEmail },
          ]}
        >
          <Input prefix={<Mail size={14} strokeWidth={1.7} />} placeholder={t.users.email} />
        </Form.Item>
        <Form.Item name="full_name" label={t.users.fullName}>
          <Input placeholder={t.users.fullName} />
        </Form.Item>
        <Form.Item
          name="password"
          label={t.users.password}
          rules={[
            { required: true, message: t.login.passwordRequired },
            { min: 6, message: t.users.passwordMinLength },
          ]}
        >
          <Input.Password prefix={<Lock size={14} strokeWidth={1.7} />} placeholder={t.users.password} />
        </Form.Item>
        <Form.Item name="is_admin" label={t.users.role} valuePropName="checked">
          <Switch checkedChildren={t.users.admin} unCheckedChildren={t.users.user} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default CreateUserModal;
