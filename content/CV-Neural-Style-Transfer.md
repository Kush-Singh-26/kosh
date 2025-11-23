---
title: "8. Computer Vision: Neural Style Transfer"
description: "Mini Project performing style transfer from a source image to a target image."
tags: ["Computer Vision"]
date: 2025-03-28
pinned: false
---

# Neural Style Transfer

- Generate new images that are combination of :
    - **Content** of one image
    - **Style** of another image

- Using `VGG19` pretrained model.

- **Content Loss** : Measures how different the content representation of the generated image is from the content representation of the content image (at a chosen deeper layer).
- **Style Loss** : Measures how different the style representation of the generated image is from the style representation of the style image (calculated across multiple layers).

- For optimization, a cloned image of the content image is iteratively updated using gradient descent.
    - modify the pixels of the generated image to minimize a weighted sum of the content loss and the style loss.

## Calculation of Losses

### Content Loss

- It is the Mean Squared Error (MSE) Loss between target image features and generated image features at a single deeper layer.
- Ensures that the generated image has similar high-level content as the content image.


### Style Loss 

- #### Gram Matrix :
    - Reshapes a tensor of an intermediate feature map of form (b,c,h,w) to form (c, h*w)
    - Matrix multiplication of this reshaped matrix with its transpose.
    - Normalize the matrix.    
<br>

- <img src="/static/images/NST1.png" alt="Image" width="750" height="300">


- The Gram matrix captures the intensity and co-occurrence of features, not their locations.
- The style loss for a single layer l is the MSE between the target and generated Gram matrices.

### Total Loss
- Weighted combination of content and style loss
- $$ L_{total} \, = \, \alpha * L_{content} \, + \, \beta * L_{style} $$

### For Inference :

- The trained gram matrix is used for getting the style.
- Thus, only the style loss if used.

> > Colab Notebook with the complete implementation can be accessed [here](https://github.com/Kush-Singh-26/Neural-Style-Transfer/blob/main/Neural_Style_Transfer.ipynb)

> - Live Implementation can be accessed [here](https://huggingface.co/spaces/Kush26/Neural_Style_Transfer).
>- It runs on cpu, so it will take a lot of time.
>    - Thus, size of image is reduced to (256, 256).
>    - Number of steps in inference is also reduced to just 100.