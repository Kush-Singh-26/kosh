---
title: "2. Computer Vision: Pre trained Models"
description: "A small guide on how to work with pre-trained models."
tags: ["Computer Vision"]
date: 2025-03-22
pinned: false
---

#### Using a pretrained model to perform classification tasks.

## Some pre-trained models available in `torchvision.models` :

- **AlexNet**: The first CNN based model to win the ImageNet challenge.

- **ResNet (Residual Networks)**: Great for deep architectures.

- **VGG (Visual Geometry Group)**: Simple but computationally expensive.

- **EfficientNet**: Balances accuracy and efficiency.

- **DenseNet**: Uses feature reusability.

# Steps for using pretrained models

### 1. Load a Pre-trained Model

```python
import torch
import torchvision.models as models

model = models.resnet50(pretrained=True)
model.eval()
```

- `pretrained=True` : the model will come with pre-learned weights from ImageNet.
- `model.eval()` : _disables dropout and batch normalization updates_.
    - this ensures that the model behaves consistently during inference (prediction).

    - if the model was used for training then `model.train()`.

> `model = models.resnet50(weights='DEFAULT')` <br>
It is the syntax for newer PyTorch versions.

### 2. Preprocess the Input Image

```python
from torchvision import transforms
from PIL import Image

transform = transforms.Compose([
    transforms.Resize(256),
    transforms.CenterCrop(224),
    transforms.ToTensor(),
    transforms.Normalize(mean = [0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
])
```

- `PIL.Image` : used to open and convert the images in the format that PyTorch can process.
- `Resize(256)` : resize the image to 256 pixels, maintaining the aspect ratio.
- `CenterCrop(224)` : Crop the image to 224×224 pixels about the center.
- `ToTensor()` : converts the image from PIL format to `Pytorch Tensor` data type.
- `Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])` 
    - Normalizes the image with ImageNet mean and std, making sure values are in a similar range as the training images.
    - It is common to use ImageNet mean and std.

```python
image_path = "example.jpg"
image = Image.open(image_path).convert("RGB")
input_tensor = transform(image).unsqueeze(0)
```

- `.unsqueeze(0)` : Adds an extra dimension (batch size), since the model expects (batch_size, C, H, W).

### 3. Forward pass for prediction

```python
with torch.no_grad():
    output = model(input_tensor) # perform forward pass

probabilities = torch.nn.functional.softmax(output[0], dim=0)
top_class = torch.argmax(probabilities).item()
```

- `with torch.no_grad()`
    - there is no need for storing gradients in inference.
    - thus saves memory and improves speed.

- `output = model(input_tensor)`
    - performs forward pass. 
    - outputs raw scores (logits) for each of the 1000 ImageNet classes.

- `.softmax(output[0], dim=0)`
    - shape of output = `(1, 1000)`
    - `output[0]` : selects the first and the only row.
        - Now the shape of output tensor = `(1000,)`.
        - Thus, only one axis remains `dimension = 0`
    - `dim=0` : tells to apply `softmax` across all 1000 values
        - the sum will be 1.

- `torch.argmax(probabilities).item()`
    - Finds the index of the class with the highest probability.
    - `.item()` extracts the number from the PyTorch tensor.`

### 4. Decode Probabilities

```python
import requests

url = "https://raw.githubusercontent.com/pytorch/hub/master/imagenet_classes.txt"
response = requests.get(url)
labels = response.text.splitlines()

class_name = labels[top_class]
print(f"Predicted class: {class_name}")
```

- labels are fetched from the url
- predicted class name is found out  

> Colab Notebook of the above code can be accessed [here](https://github.com/Kush-Singh-26/Learning-Pytorch/blob/main/Pretrainedmodel.ipynb)
